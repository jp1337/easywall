"""
TODO: Doku
"""
from datetime import datetime
from logging import debug, info

from easywall.acceptance import Acceptance
from easywall.config import Config
from easywall.iptables_handler import Iptables
from easywall.rules_handler import RulesHandler
from easywall.utility import file_exists, rename_file


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
        self.rules = RulesHandler()

    def apply(self) -> None:
        """
        TODO: Doku
        """
        self.acceptance.start()
        self.rotate_backup()
        self.iptables.save()
        self.rules.backup_current_rules()
        self.rules.apply_new_rules()
        self.apply_iptables()
        self.acceptance.wait()

        if self.acceptance.status() == "not accepted":
            self.iptables.restore()
            self.rules.rollback_from_backup()
            info("Configuration was not accepted, rollback applied")
        else:
            info("New configuration was applied.")

    def apply_iptables(self) -> None:
        """
        TODO: Doku
        """
        # and reset iptables for clean setup
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

        # Apply Custom Rules
        self.apply_custom_rules()

        # log and reject all other packages
        self.iptables.add_append("INPUT", "-j LOG --log-prefix \" easywall[other]: \"")
        self.iptables.add_append("INPUT", "-j REJECT")

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

    def apply_blacklist(self) -> None:
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

    def apply_whitelist(self) -> None:
        """
        this function adds rules to iptables which explicitly allows a connection
        from this list ip addresses
        """
        for ipaddr in self.rules.get_current_rules("whitelist"):
            if ":" in ipaddr:
                self.iptables.add_append("INPUT", "-s {} -j ACCEPT".format(ipaddr), onlyv6=True)
            else:
                self.iptables.add_append("INPUT", "-s {} -j ACCEPT".format(ipaddr), onlyv4=True)

    def apply_rules(self, ruletype) -> None:
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

    def apply_custom_rules(self) -> None:
        """
        TODO: Doku
        """
        for rule in self.rules.get_current_rules("custom"):
            self.iptables.add_append(
                chain="INPUT",
                rule=rule
            )

    def rotate_backup(self) -> None:
        """
        TODO: Doku
        """
        self.filepath = self.cfg.get_value("BACKUP", "filepath")
        self.filename = self.cfg.get_value("BACKUP", "ipv4filename")
        self.date = datetime.now().strftime("%Y-%m-%d_%H-%M-%S")

        self.rename_backup_file()
        if self.ipv6 is True:
            self.filename = self.cfg.get_value("BACKUP", "ipv6filename")
            self.rename_backup_file()

        debug("backup file rotated in folder {} \n prefix added: {}".format(
            self.filepath, self.date))

    def rename_backup_file(self) -> None:
        """
        TODO: Doku
        """
        old_filename = "{}/{}".format(self.filepath, self.filename)
        new_filename = "{}/{}_{}".format(self.filepath, self.date, self.filename)
        if file_exists(old_filename):
            rename_file(old_filename, new_filename)
