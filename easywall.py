import os
import config
import log
from enum import Enum


class ruletype(Enum):
    TCP = 1
    UDP = 2


class iptables(object):
    def __init__(self):
        self.config = config.config("config/config.ini")
        self.iptables = self.config.getValue("EXEC", "IPTables")
        self.ip6tables = self.config.getValue("EXEC", "IP6Tables")
        self.iptables_save = self.config.getValue("EXEC", "IPTables-save")
        self.ip6tables_save = self.config.getValue("EXEC", "IP6Tables-save")

    def addrule(self, port, ruletype):
        if ruletype == ruletype.TCP:
            os.system(self.iptables + " -A INPUT -p tcp --dport " +
                      str(port) + " --syn -m conntrack --ctstate NEW -j ACCEPT")
        elif ruletype == ruletype.UCP:
            os.system(self.iptables + " -A INPUT -p udp --dport " +
                      str(port) + " -m conntrack --ctstate NEW -j ACCEPT")
        else:
            log.logging.error(
                "Ruletype does not match any known ruletype. Ruletype given: " + ruletype)

    def reset(self):
        log.logging.info(
            "Beginning to reset the firewall rules and opening the firewall for every connection")

        os.system(self.iptables + " -P INPUT ACCEPT")
        os.system(self.iptables + " -P OUTPUT ACCEPT")
        os.system(self.iptables + " -P FORWARD ACCEPT")
        os.system(self.iptables + " -F")
        os.system(self.iptables + " -X")
        os.system(self.iptables + " -t nat -F")
        os.system(self.iptables + " -t nat -X")
        os.system(self.iptables + " -t mangle -F")
        os.system(self.iptables + " -t mangle -X")

        os.system(self.ip6tables + " -P INPUT ACCEPT")
        os.system(self.ip6tables + " -P OUTPUT ACCEPT")
        os.system(self.ip6tables + " -P FORWARD ACCEPT")
        os.system(self.ip6tables + " -F")
        os.system(self.ip6tables + " -X")
        os.system(self.ip6tables + " -t nat -F")
        os.system(self.ip6tables + " -t nat -X")
        os.system(self.ip6tables + " -t mangle -F")
        os.system(self.ip6tables + " -t mangle -X")

        log.logging.info(
            "The firewall rules for IPV4 and IPV6 have been successfully reset")

    def list(self):
        os.system(self.iptables + " -L")

    def save(self):
        os.system(self.iptables_save + " -L")


class easywall(object):
    def __init__(self):
        self.log = log.log()
        self.iptables = iptables()
        self.iptables.reset()
        self.iptables.addrule(443, ruletype.TCP)
        self.iptables.list()
        self.log.closeLogging()

    # def apply(self):
        # Save old rules
        #


easywall()
