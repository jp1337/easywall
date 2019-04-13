import config
import log
import iptables
import acceptance
import os


class easywall(object):
    def __init__(self):
        self.log = log.log()
        log.logging.info("Starting up EasyWall...")
        self.config = config.config("config/config.ini")
        self.iptables = iptables.iptables()
        self.acceptance = acceptance.acceptance()
        self.apply()  # only testing
        log.logging.info("Shutting down EasyWall...")
        self.log.closeLogging()

    def apply(self):
        self.acceptance.reset()
        self.acceptance.accept()  # testing only!!!

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
        # Allow IP-addresses from whitelist

        # Allow TCP Ports
        for port in self.getRules("tcp"):
            if port != "":
                if ":" in port:
                    self.iptables.addAppend(
                        "INPUT", "-p tcp --match multiport --dports " + port + " -m conntrack --ctstate NEW -j ACCEPT")
                else:
                    self.iptables.addAppend(
                        "INPUT", "-p tcp --dport " + port + " -m conntrack --ctstate NEW -j ACCEPT")

        # Allow UDP Ports

        # log and reject all other packages
        self.iptables.addAppend("INPUT", "-j LOG")
        self.iptables.addAppend("INPUT", "-j REJECT")

        # self.iptables.list()
        if self.acceptance.check() == False:
            self.iptables.restore()
        else:
            # move old rules
            # save new rules
            print("")
        self.iptables.list()

    def getRules(self, ruletype):
        self.filepath = self.config.getValue("RULES", "filepath")
        self.filename = self.config.getValue("RULES", ruletype)

        if not os.path.exists(self.filepath):
            os.makedirs(self.filepath)
        with open(self.filepath + "/" + self.filename, 'r+') as rulesfile:
            lines = rulesfile.read().split('\n')
            return lines


if __name__ == "__main__":
    easywall()
