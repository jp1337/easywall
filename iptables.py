import config
import os
import log
import utility
from enum import Enum


class ruletype(Enum):
    TCP = 1
    UDP = 2


class iptables(object):
    def __init__(self):
        self.config = config.config("config/config.ini")
        self.ipv6 = bool(self.config.getValue("IPV6", "enabled"))
        self.iptables = self.config.getValue("EXEC", "iptables")
        self.iptables_save = self.config.getValue("EXEC", "iptables-save")
        self.iptables_restore = self.config.getValue(
            "EXEC", "iptables-restore")
        if self.ipv6 == True:
            self.ip6tables = self.config.getValue("EXEC", "ip6tables")
            self.ip6tables_save = self.config.getValue(
                "EXEC", "ip6tables-save")
            self.ip6tables_restore = self.config.getValue(
                "EXEC", "ip6tables-restore")

    def addPolicy(self, chain, target):
        if target == "ACCEPT" or target == "DROP":
            os.system(self.iptables + " -P " + chain + " " + target)
            if self.ipv6 == True:
                os.system(self.ip6tables + " -P " + chain + " " + target)
        else:
            log.logging.error("Invalid Target for addPolicy " + target)

    def addChain(self, chain):
        os.system(self.iptables + " -N " + chain)
        if self.ipv6 == True:
            os.system(self.ip6tables + " -N " + chain)

    def addAppend(self, chain, rule, onlyv6=False, onlyv4=False):
        if onlyv4 == True and onlyv6 == False:
            os.system(self.iptables + " -A " + chain + " " + rule)
        if self.ipv6 == True and onlyv6 == True:
            os.system(self.ip6tables + " -A " + chain + " " + rule)

    def flush(self, chain=""):
        os.system(self.iptables + " -F " + chain)
        if self.ipv6 == True:
            os.system(self.ip6tables + " -F " + chain)

    def deleteChain(self, chain=""):
        os.system(self.iptables + " -X " + chain)
        if self.ipv6 == True:
            os.system(self.ip6tables + " -X " + chain)

    def reset(self):
        log.logging.info(
            "Beginning to reset the firewall rules and opening the firewall for every connection")
        self.addPolicy("INPUT", "ACCEPT")
        self.addPolicy("OUTPUT", "ACCEPT")
        self.addPolicy("FORWARD", "ACCEPT")
        self.flush()
        self.deleteChain()
        log.logging.info(
            "The firewall rules for IPV4 and IPV6 have been successfully reset")

    def list(self):
        os.system(self.iptables + " -L")

    def save(self):
        # Create Backup Directory if not exists
        filepath = self.config.getValue("BACKUP", "filepath")
        utility.createFolderIfNotExists(filepath)

        # backing up ipv4 iptables rules
        filename = self.config.getValue("BACKUP", "ipv4filename")
        with open(filepath + "/" + filename, 'w'):
            pass
        os.system(self.iptables_save + " | while read IN ; do echo $IN >> " +
                  filepath + "/" + filename + " ; done")

        # backing up ipv6 iptables rules
        if self.ipv6 == True:
            filename = self.config.getValue("BACKUP", "ipv6filename")
            with open(filepath + "/" + filename, 'w'):
                pass
            os.system(self.ip6tables_save + " | while read IN ; do echo $IN >> " +
                      filepath + "/" + filename + " ; done")

    def restore(self):
        log.logging.info("Starting Firewall Rule Restore...")
        filepath = self.config.getValue("BACKUP", "filepath")
        utility.createFolderIfNotExists(filepath)

        log.logging.info("Restoring ipv4 rules...")
        filename = self.config.getValue("BACKUP", "ipv4filename")
        os.system(self.iptables_restore + " < " +
                  filepath + "/" + filename)

        if self.ipv6 == True:
            log.logging.info("Restoring ipv6 rules...")
            filename = self.config.getValue("BACKUP", "ipv6filename")
            os.system(self.ip6tables_restore + " < " +
                      filepath + "/" + filename)
