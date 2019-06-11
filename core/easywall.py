import config
import log
import iptables
import acceptance
import os
import utility
import time
from datetime import datetime
from watchdog.observers import Observer
from watchdog.events import FileSystemEventHandler


class ModifiedHandler(FileSystemEventHandler):
    def on_any_event(self, event):
        if event.src_path.endswith(".txt"):
            log.logging.info(
                "file modification occured. filename: " + event.src_path)
            while os.path.isfile(".running"):
                time.sleep(1)
            easywall()


class easywall(object):

    def __init__(self):
        log.logging.info("Applying new configuration.")
        self.create_running_file()
        self.config = config.config("config/config.ini")
        self.iptables = iptables.iptables()
        self.acceptance = acceptance.acceptance()
        self.apply()
        self.delete_running_file()

    def apply(self):
        self.acceptance.reset()

        # save current ruleset and reset iptables for clean setup
        self.iptables.save()
        self.iptables.reset()

        # drop intbound traffic and allow outbound traffic
        self.iptables.addPolicy("INPUT", "DROP")
        self.iptables.addPolicy("OUTPUT", "ACCEPT")

        # allow loopback access
        self.iptables.addAppend("INPUT", "-i lo -j ACCEPT")

        # allow established or related connections
        self.iptables.addAppend(
            "INPUT", "-m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT")

        # Block remote packets claiming to be from a loopback address.
        self.iptables.addAppend(
            "INPUT", "-s 127.0.0.0/8 ! -i lo -j DROP", False, True)
        self.iptables.addAppend("INPUT", "-s ::1/128 ! -i lo -j DROP", True)

        # Block IP-addresses from blacklist
        self.apply_blacklist()

        # Allow IP-addresses from whitelist
        self.apply_whitelist()

        # Allow TCP Ports
        self.apply_rules("tcp")

        # Allow UDP Ports
        self.apply_rules("udp")

        # log and reject all other packages
        self.iptables.addAppend(
            "INPUT", "-j LOG --log-prefix \" easywall[other]: \"")
        self.iptables.addAppend("INPUT", "-j REJECT")

        self.check_acceptance()

    def apply_blacklist(self):
        for ip in self.get_rule_list("blacklist"):
            if ":" in ip:
                self.iptables.addAppend(
                    "INPUT", "-s " + ip +
                    " -j LOG --log-prefix \" easywall[blacklist]: \"", True)
                self.iptables.addAppend(
                    "INPUT", "-s " + ip + " -j DROP", True)
            else:
                self.iptables.addAppend(
                    "INPUT", "-s " + ip +
                    " -j LOG --log-prefix \" easywall[blacklist]: \"", False,
                    True)
                self.iptables.addAppend(
                    "INPUT", "-s " + ip + " -j DROP", False, True)

    def apply_whitelist(self):
        for ip in self.get_rule_list("whitelist"):
            if ":" in ip:
                self.iptables.addAppend(
                    "INPUT", "-s " + ip + " -j ACCEPT", True)
            else:
                self.iptables.addAppend(
                    "INPUT", "-s " + ip + " -j ACCEPT", False, True)

    def apply_rules(self, ruletype):
        for port in self.get_rule_list(ruletype):
            if ":" in port:
                self.iptables.addAppend(
                    "INPUT", "-p " + ruletype +
                    " --match multiport --dports " + port +
                    " -m conntrack --ctstate NEW -j ACCEPT")
            else:
                self.iptables.addAppend(
                    "INPUT", "-p " + ruletype + " --dport " + port +
                    " -m conntrack --ctstate NEW -j ACCEPT")

    def check_acceptance(self):
        log.logging.info("Checking acceptance.")
        if self.acceptance.check() is False:
            log.logging.info("Configuration not accepted, rolling back.")
            self.iptables.restore()
        else:
            self.rotate_backup()
            self.iptables.save()
            log.logging.info("New configuration was applied.")

    def get_rule_list(self, ruletype):
        rule_list = []
        with open(self.config.getValue("RULES", "filepath") + "/" +
                  self.config.getValue("RULES", ruletype), 'r') as rulesfile:
            for rule in rulesfile.read().split('\n'):
                if rule != "":
                    rule_list.append(rule)
        return rule_list

    def rotate_backup(self):
        self.filepath = self.config.getValue("BACKUP", "filepath")
        self.filename = self.config.getValue("BACKUP", "ipv4filename")
        self.date = datetime.now().strftime("%Y-%m-%d_%H-%M-%S")
        log.logging.debug("rotating backup files in folder " +
                          self.filepath + " -> add prefix " + self.date)
        os.rename(self.filepath + "/" + self.filename,
                  self.filepath + "/" + self.date + "_" + self.filename)
        self.ipv6 = self.config.getValue("IPV6", "enabled")
        if bool(self.ipv6) is True:
            self.filenamev6 = self.config.getValue("BACKUP", "ipv6filename")
            os.rename(self.filepath + "/" + self.filenamev6,
                      self.filepath + "/" + self.date + "_" + self.filenamev6)

    def create_running_file(self):
        utility.create_file_if_not_exists(".running")

    def delete_running_file(self):
        utility.delete_file_if_exists(".running")


def run():
    # Startup Process
    masterlog = log.log()
    log.logging.info("Starting up EasyWall...")
    masterconfig = config.config("config/config.ini")
    ensure_rules_files(masterconfig)
    event_handler = ModifiedHandler()
    observer = Observer()
    observer.schedule(
        event_handler, masterconfig.getValue("RULES", "filepath"))
    observer.start()
    log.logging.info("EasyWall is up and running.")

    # waiting for file modifications
    try:
        while True:
            time.sleep(1)
    except Exception:
        shutdown(observer, masterconfig, masterlog)
    shutdown(observer, masterconfig, masterlog)


def shutdown(observer, masterconfig, masterlog):
    # Shutdown Process
    log.logging.info("Shutting down EasyWall...")
    observer.stop()
    utility.delete_file_if_exists(".running")
    utility.delete_file_if_exists(
        masterconfig.getValue("ACCEPTANCE", "filename"))
    observer.join()
    masterlog.closeLogging()
    log.logging.info("EasyWall was stopped gracefully")


def ensure_rules_files(config):
    for ruletype in ["blacklist", "whitelist", "tcp", "udp"]:
        filepath = config.getValue("RULES", "filepath")
        filename = config.getValue("RULES", ruletype)
        utility.create_folder_if_not_exists(filepath)
        utility.create_file_if_not_exists(filepath + "/" + filename)


if __name__ == "__main__":
    run()
