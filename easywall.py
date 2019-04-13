import os
import config
import log
from time import sleep
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
        # Create Backup Directory if not exists
        filepath = self.config.getValue("BACKUP", "filepath")
        if not os.path.exists(filepath):
            os.makedirs(filepath)

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
            os.system(self.iptables_save + " | while read IN ; do echo $IN >> " +
                      filepath + "/" + filename + " ; done")

    def restore(self):
        filepath = self.config.getValue("BACKUP", "filepath")
        log.logging.info("Starting Firewall Rule Restore...")

        # Check if Backup dir exists
        if os.path.exists(filepath):
            log.logging.info("Restoring ipv4 rules...")
            filename = self.config.getValue("BACKUP", "ipv4filename")
            os.system(self.iptables_restore + " -v < " +
                      filepath + "/" + filename)

            if self.ipv6 == True:
                log.logging.info("Restoring ipv6 rules...")
                filename = self.config.getValue("BACKUP", "ipv6filename")
                os.system(self.ip6tables_restore + " -v < " +
                          filepath + "/" + filename)


class easywall(object):
    def __init__(self):
        self.log = log.log()
        log.logging.info("Starting up EasyWall...")
        self.iptables = iptables()
        self.acceptance = acceptance()
        self.apply()  # only testing
        log.logging.info("Shutting down EasyWall...")
        self.log.closeLogging()

    def apply(self):
        self.acceptance.reset()
        self.iptables.save()
        self.iptables.reset()
        # drop incoming connections && accept outgoing connections
        # allow loopback access
        # allow established or related connections
        # Block remote packets claiming to be from a loopback address.
        # Block IP-addresses from blacklist
        # Allow IP-addresses from whitelist
        # Allow TCP Ports
        # Allow UDP Ports
        # Drop and Log invalid packages
        # Final Rules
        self.iptables.list()
        if self.acceptance.check() == False:
            self.iptables.restore()
        else:
            # move old rules
            # save new rules
            print("")
        self.iptables.list()


class acceptance(object):
    def __init__(self):
        self.config = config.config("config/config.ini")
        self.filename = self.config.getValue("ACCEPTANCE", "filename")

    def reset(self):
        with open(self.filename, 'w') as accfile:
            accfile.write('false')

    def check(self):
        seconds = self.config.getValue("ACCEPTANCE", "time")
        log.logging.info(
            "Starting Acceptance Check... waiting for " + seconds + " seconds")
        sleep(int(seconds))
        with open(self.filename, 'r') as accfile:
            accepted = accfile.read()
            if accepted == "true":
                log.logging.info("Acceptance Process Result: Accepted")
                return True
            else:
                log.logging.info(
                    "Acceptance Process Result: Not Accepted (file content: " + accepted + ")")
                return False


if __name__ == "__main__":
    easywall()
