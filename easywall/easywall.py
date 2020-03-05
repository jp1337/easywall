"""this is the module which interacts with the given firewall"""
from datetime import datetime
from logging import debug, info

from easywall.acceptance import Acceptance
from easywall.config import Config
from easywall.iptables_handler import Iptables
from easywall.utility import rename_file
from easywall.rules_handler import RulesHandler


class Easywall(object):
    """
    the class contains the main functions for the easywall core
    such as applying a new configuration or listening on rule file changes
    """

    def __init__(self, config: Config) -> None:
        self.cfg = config
        self.iptables = Iptables(self.cfg)
        self.acceptance = Acceptance(self.cfg)
        self.ipv6 = self.cfg.get_value("IPV6", "enabled")
        self.filepath = None
        self.filename = None
        self.date = None
        self.rules = RulesHandler(self.cfg)

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
        self.iptables.add_append("INPUT", "-m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT")

        # Block remote packets claiming to be from a loopback address.
        self.iptables.add_append("INPUT", "-s 127.0.0.0/8 ! -i lo -j DROP", False, True)
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
        self.iptables.add_append("INPUT", "-j LOG --log-prefix \" easywall[other]: \"")
        self.iptables.add_append("INPUT", "-j REJECT")

        self.check_acceptance()

    def apply_icmp(self) -> None:
        """
        this function adds rules to iptables for incoming ICMP requests
        """
        for icmptype in [0, 3, 8, 11]:
            self.iptables.add_append(
                "INPUT", "-p icmp --icmp-type {} -m conntrack --ctstate NEW -j ACCEPT".format(
                    icmptype), False, True)
        if self.ipv6 is True:
            for icmptype in [1, 2, 3, 4, 128, 133, 134, 135, 136, 137, 141, 142, 151, 152, 153]:
                self.iptables.add_append(
                    "INPUT", "-p ipv6-icmp --icmpv6-type {} -j ACCEPT".format(icmptype), True)

    def apply_blacklist(self):
        """
        this function adds rules to iptables which block incoming traffic
        from a list of ip addresses
        """
        for ipaddr in self.rules.get_current_rules("blacklist"):
            if ":" in ipaddr:
                self.iptables.add_append(
                    chain="INPUT",
                    rule="-s {} -j LOG --log-prefix \" easywall[blacklist]: \"".format(ipaddr),
                    onlyv6=True)
                self.iptables.add_append("INPUT", "-s {} -j DROP".format(ipaddr), onlyv6=True)
            else:
                self.iptables.add_append(
                    chain="INPUT",
                    rule="-s {} -j LOG --log-prefix \" easywall[blacklist]: \"".format(ipaddr),
                    onlyv4=True)
                self.iptables.add_append("INPUT", "-s {} -j DROP".format(ipaddr), onlyv4=True)

    def apply_whitelist(self):
        """
        this function adds rules to iptables which explicitly allows a connection
        from this list ip addresses
        """
        for ipaddr in self.rules.get_current_rules("whitelist"):
            if ":" in ipaddr:
                self.iptables.add_append("INPUT", "-s {} -j ACCEPT".format(ipaddr), onlyv6=True)
            else:
                self.iptables.add_append("INPUT", "-s {} -j ACCEPT".format(ipaddr), onlyv4=True)

    def apply_rules(self, ruletype):
        """
        this function adds rules for incoming tcp and udp connections to iptables
        which allow a connection to this list of ports

        [INFO] the function also processes port ranges split by ":" separator.
        """
        for port in self.rules.get_current_rules(ruletype):
            if ":" in port:
                rule = "-p {} --match multiport --dports {}".format(
                    ruletype, port)
            else:
                rule = "-p {} --dport {}".format(ruletype, port)

            self.iptables.add_append(
                chain="INPUT",
                rule="{} -m conntrack --ctstate NEW -j ACCEPT".format(rule)
            )

    def check_acceptance(self):
        """the function checks for accetance of the new applied configuration"""
        info("Checking acceptance.")
        if self.acceptance.check() is False:
            info("Configuration not accepted, rolling back.")
            self.iptables.restore()
        else:
            self.rotate_backup()
            self.iptables.save()
            info("New configuration was applied.")

    def rotate_backup(self):
        """the function rotates the backup files to have a clean history of files"""
        self.filepath = self.cfg.get_value("BACKUP", "filepath")
        self.filename = self.cfg.get_value("BACKUP", "ipv4filename")
        self.date = datetime.now().strftime("%Y-%m-%d_%H-%M-%S")
        debug("rotating backup files in folder " +
              self.filepath + " -> add prefix " + self.date)
        self.rename_backup_file()
        if self.ipv6 is True:
            self.filename = self.cfg.get_value("BACKUP", "ipv6filename")
            self.rename_backup_file()

    def rename_backup_file(self):
        """the function renames a backup file"""
        rename_file("{}/{}".format(self.filepath, self.filename),
                    "{}/{}_{}".format(self.filepath, self.date, self.filename))
