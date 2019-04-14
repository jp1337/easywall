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
                "file modification occured. infos: " + event.src_path)
            while os.path.isfile(".running"):
                time.sleep(1)
            easywall()


class easywall(object):

    def __init__(self):
        self.createRunningFile()
        self.config = config.config("config/config.ini")
        self.iptables = iptables.iptables()
        self.acceptance = acceptance.acceptance()
        self.apply()
        self.deleteRunningFile()

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
        for ip in self.getRuleList("blacklist"):
            if ip != "":
                if ":" in ip:
                    self.iptables.addAppend(
                        "INPUT", "-s " + ip + " -j LOG --log-prefix \" easywall[blacklist]: \"", True)
                    self.iptables.addAppend(
                        "INPUT", "-s " + ip + " -j DROP", True)
                else:
                    self.iptables.addAppend(
                        "INPUT", "-s " + ip + " -j LOG --log-prefix \" easywall[blacklist]: \"", False, True)
                    self.iptables.addAppend(
                        "INPUT", "-s " + ip + " -j DROP", False, True)

        # Allow IP-addresses from whitelist
        for ip in self.getRuleList("whitelist"):
            if ip != "":
                if ":" in ip:
                    self.iptables.addAppend(
                        "INPUT", "-s " + ip + " -j ACCEPT", True)
                else:
                    self.iptables.addAppend(
                        "INPUT", "-s " + ip + " -j ACCEPT", False, True)

        # Allow TCP Ports
        for port in self.getRuleList("tcp"):
            if port != "":
                if ":" in port:
                    self.iptables.addAppend(
                        "INPUT", "-p tcp --match multiport --dports " + port + " -m conntrack --ctstate NEW -j ACCEPT")
                else:
                    self.iptables.addAppend(
                        "INPUT", "-p tcp --dport " + port + " -m conntrack --ctstate NEW -j ACCEPT")

        # Allow UDP Ports
        for port in self.getRuleList("udp"):
            if port != "":
                if ":" in port:
                    self.iptables.addAppend(
                        "INPUT", "-p udp --match multiport --dports " + port + " -m conntrack --ctstate NEW -j ACCEPT")
                else:
                    self.iptables.addAppend(
                        "INPUT", "-p udp --dport " + port + " -m conntrack --ctstate NEW -j ACCEPT")

        # log and reject all other packages
        self.iptables.addAppend(
            "INPUT", "-j LOG --log-prefix \" easywall[other]: \"")
        self.iptables.addAppend("INPUT", "-j REJECT")

        if self.acceptance.check() == False:
            self.iptables.restore()
        else:
            self.rotateRules()
            self.iptables.save()

    def getRuleList(self, ruletype):
        with open(self.config.getValue("RULES", "filepath") + "/" + self.config.getValue("RULES", ruletype), 'r') as rulesfile:
            return rulesfile.read().split('\n')

    def rotateRules(self):
        self.filepath = self.config.getValue("BACKUP", "filepath")
        self.filename = self.config.getValue("BACKUP", "ipv4filename")
        self.date = datetime.now().strftime("%Y-%m-%d_%H-%M-%S")
        os.rename(self.filepath + "/" + self.filename,
                  self.filepath + "/" + self.date + "_" + self.filename)
        self.ipv6 = self.config.getValue("IPV6", "enabled")
        if bool(self.ipv6) == True:
            self.filename = self.config.getValue("BACKUP", "ipv6filename")
            os.rename(self.filepath + "/" + self.filename,
                      self.filepath + "/" + self.date + "_" + self.filename)

    def createRunningFile(self):
        utility.createFileIfNotExists(".running")

    def deleteRunningFile(self):
        utility.deleteFileIfExists(".running")


def run():
    # Startup Process
    masterlog = log.log()
    log.logging.info("Starting up EasyWall...")
    masterconfig = config.config("config/config.ini")
    ensureRulesFiles(masterconfig)
    event_handler = ModifiedHandler()
    observer = Observer()
    observer.schedule(
        event_handler, masterconfig.getValue("RULES", "filepath"))
    observer.start()
    log.logging.info("EasyWall is up and running.")

    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        observer.stop()

    # Shutdown Process
    log.logging.info("Shutting down EasyWall...")
    utility.deleteFileIfExists(".running")
    utility.deleteFileIfExists(masterconfig.getValue("ACCEPTANCE", "filename"))
    observer.join()
    masterlog.closeLogging()


def ensureRulesFiles(config):
    for ruletype in ["blacklist", "whitelist", "tcp", "udp"]:
        filepath = config.getValue("RULES", "filepath")
        filename = config.getValue("RULES", ruletype)
        utility.createFolderIfNotExists(filepath)
        utility.createFileIfNotExists(filepath + "/" + filename)


if __name__ == "__main__":
    run()
