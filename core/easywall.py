"""the module contains the core functions of easywall"""
import os
import time
from datetime import datetime

from watchdog.events import FileSystemEventHandler
from watchdog.observers import Observer

import log
import utility
from acceptance import Acceptance
from config import Config
from iptables import Iptables


class ModifiedHandler(FileSystemEventHandler):
    """the class contains a event handler which listens on specified file types"""

    def on_any_event(self, event):
        """
        the function overrides the empty event handler for every file change in the given directory
        """
        if event.src_path.endswith(".txt"):
            log.logging.info(
                "file modification occured. filename: " + event.src_path)
            while os.path.isfile(".running"):
                time.sleep(2)
            Easywall()


class Easywall(object):
    """
    the class contains the main functions for the easywall core
    such as applying a new configuration or listening on rule file changes
    """

    def __init__(self):
        log.logging.info("Applying new configuration.")
        self.create_running_file()
        self.config = Config("config/config.ini")
        self.iptables = Iptables()
        self.acceptance = Acceptance()
        self.ipv6 = self.config.get_value("IPV6", "enabled")
        self.filepath = None
        self.filename = None
        self.date = None
        self.apply()
        self.delete_running_file()

    def apply(self):
        """the function applies the configuration from the rule files"""
        self.acceptance.reset()

        # save current ruleset and reset iptables for clean setup
        self.iptables.save()
        self.iptables.reset()

        # drop intbound traffic and allow outbound traffic
        self.iptables.add_policy("INPUT", "DROP")
        self.iptables.add_policy("OUTPUT", "ACCEPT")

        # allow loopback access
        self.iptables.add_append("INPUT", "-i lo -j ACCEPT")

        # allow established or related connections
        self.iptables.add_append(
            "INPUT", "-m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT")

        # Block remote packets claiming to be from a loopback address.
        self.iptables.add_append(
            "INPUT", "-s 127.0.0.0/8 ! -i lo -j DROP", False, True)
        self.iptables.add_append("INPUT", "-s ::1/128 ! -i lo -j DROP", True)

        # Apply ICMP Rules
        self.apply_icmp()

        # Block IP-addresses from blacklist
        self.apply_blacklist()

        # Allow IP-addresses from whitelist
        self.apply_whitelist()

        # Allow TCP Ports
        self.apply_rules("tcp")

        # Allow UDP Ports
        self.apply_rules("udp")

        # log and reject all other packages
        self.iptables.add_append(
            "INPUT", "-j LOG --log-prefix \" easywall[other]: \"")
        self.iptables.add_append("INPUT", "-j REJECT")

        self.check_acceptance()

    def apply_icmp(self):
        """the function applies the icmp rules"""
        for icmptype in [0, 3, 8, 11]:
            self.iptables.add_append(
                "INPUT", "-p icmp --icmp-type " + str(icmptype) +
                " -m conntrack --ctstate NEW -j ACCEPT", False, True)
        if self.ipv6 is True:
            for icmptype in [
                    1, 2, 3, 4, 128, 133, 134, 135, 136, 137,
                    141, 142, 151, 152, 153]:
                self.iptables.add_append(
                    "INPUT", "-p ipv6-icmp --icmpv6-type " +
                    str(icmptype) + " -j ACCEPT", True)

    def apply_blacklist(self):
        """the function applies the blacklist rules from the rules file"""
        for ipaddr in self.get_rule_list("blacklist"):
            if ":" in ipaddr:
                self.iptables.add_append(
                    "INPUT", "-s " + ipaddr +
                    " -j LOG --log-prefix \" easywall[blacklist]: \"", True)
                self.iptables.add_append(
                    "INPUT", "-s " + ipaddr + " -j DROP", True)
            else:
                self.iptables.add_append(
                    "INPUT", "-s " + ipaddr +
                    " -j LOG --log-prefix \" easywall[blacklist]: \"", False,
                    True)
                self.iptables.add_append(
                    "INPUT", "-s " + ipaddr + " -j DROP", False, True)

    def apply_whitelist(self):
        """the function applies the whitelist rules from the rules file"""
        for ipaddr in self.get_rule_list("whitelist"):
            if ":" in ipaddr:
                self.iptables.add_append(
                    "INPUT", "-s " + ipaddr + " -j ACCEPT", True)
            else:
                self.iptables.add_append(
                    "INPUT", "-s " + ipaddr + " -j ACCEPT", False, True)

    def apply_rules(self, ruletype):
        """the function applies the rules from the rules file"""
        for port in self.get_rule_list(ruletype):
            if ":" in port:
                self.iptables.add_append(
                    "INPUT", "-p " + ruletype +
                    " --match multiport --dports " + port +
                    " -m conntrack --ctstate NEW -j ACCEPT")
            else:
                self.iptables.add_append(
                    "INPUT", "-p " + ruletype + " --dport " + port +
                    " -m conntrack --ctstate NEW -j ACCEPT")

    def check_acceptance(self):
        """the function checks for accetance of the new applied configuration"""
        log.logging.info("Checking acceptance.")
        if self.acceptance.check() is False:
            log.logging.info("Configuration not accepted, rolling back.")
            self.iptables.restore()
        else:
            self.rotate_backup()
            self.iptables.save()
            log.logging.info("New configuration was applied.")

    def get_rule_list(self, ruletype):
        """the function retrieves the rules from the rules file"""
        rule_list = []
        with open(self.config.get_value("RULES", "filepath") + "/" +
                  self.config.get_value("RULES", ruletype), 'r') as rulesfile:
            for rule in rulesfile.read().split('\n'):
                if rule.strip() != "":
                    rule_list.append(rule)
        return rule_list

    def rotate_backup(self):
        """the function rotates the backup files to have a clean history of files"""
        self.filepath = self.config.get_value("BACKUP", "filepath")
        self.filename = self.config.get_value("BACKUP", "ipv4filename")
        self.date = datetime.now().strftime("%Y-%m-%d_%H-%M-%S")
        log.logging.debug("rotating backup files in folder " +
                          self.filepath + " -> add prefix " + self.date)
        self.rename_backup_file()
        if self.ipv6 is True:
            self.filename = self.config.get_value("BACKUP", "ipv6filename")
            self.rename_backup_file()

    def rename_backup_file(self):
        """the function renames a backup file"""
        os.rename(self.filepath + "/" + self.filename,
                  self.filepath + "/" + self.date + "_" + self.filename)

    def create_running_file(self):
        """the function creates a file in the main directory called .running"""
        utility.create_file_if_not_exists(".running")

    def delete_running_file(self):
        """the function deletes a file in the main directory called .running"""
        utility.delete_file_if_exists(".running")


def run():
    """this is the main function of the program"""
    # Startup Process
    masterlog = log.Log("config/config.ini")
    log.logging.info("Starting up EasyWall...")
    masterconfig = Config("config/config.ini")
    ensure_rules_files(masterconfig)
    event_handler = ModifiedHandler()
    observer = Observer()
    observer.schedule(
        event_handler, masterconfig.get_value("RULES", "filepath"))
    observer.start()
    log.logging.info("EasyWall is up and running.")

    # waiting for file modifications
    try:
        while True:
            time.sleep(2)
    except KeyboardInterrupt:
        shutdown(observer, masterconfig, masterlog)
    shutdown(observer, masterconfig, masterlog)


def shutdown(observer, masterconfig, masterlog):
    """this function executes a shutdown of easywall"""
    log.logging.info("Shutting down EasyWall...")
    observer.stop()
    utility.delete_file_if_exists(".running")
    utility.delete_file_if_exists(
        masterconfig.get_value("ACCEPTANCE", "filename"))
    observer.join()
    masterlog.close_logging()
    log.logging.info("EasyWall was stopped gracefully")
    exit(0)


def ensure_rules_files(cfg):
    """the function creates several files if they don't exist"""
    for ruletype in ["blacklist", "whitelist", "tcp", "udp", "custom"]:
        filepath = cfg.get_value("RULES", "filepath")
        filename = cfg.get_value("RULES", ruletype)
        utility.create_folder_if_not_exists(filepath)
        utility.create_file_if_not_exists(filepath + "/" + filename)


if __name__ == "__main__":
    run()
