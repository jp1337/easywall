"""
TODO: Doku
"""
from easywall.config import Config
from easywall.easywall import Easywall
from easywall.utility import (create_file_if_not_exists, delete_file_if_exists,
                              write_into_file)

from tests import unittest


class TestRulesHandler(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self) -> None:
        self.config_file = "test_easywall.ini"
        content = """[LOG]
level = info
to_files = no
to_stdout = yes
filepath = /var/log
filename = easywall.log

[IPTABLES]
log_blocked_connections = yes
log_blocked_connections_log_limit = 60
log_blacklist_connections = yes
log_blacklist_connections_log_limit = 60
drop_broadcast_packets = yes
drop_multicast_packets = yes
drop_anycast_packets = yes
ssh_brute_force_prevention = yes
ssh_brute_force_prevention_log = yes
ssh_brute_force_prevention_connection_limit = 5
ssh_brute_force_prevention_log_limit = 60
icmp_flood_prevention = yes
icmp_flood_prevention_log = yes
icmp_flood_prevention_connection_limit = 5
icmp_flood_prevention_log_limit = 60
drop_invalid_packets = yes
drop_invalid_packets_log = yes
drop_invalid_packets_log_limit = 60
port_scan_prevention = yes
port_scan_prevention_log = yes
port_scan_prevention_log_limit = 60

[IPV6]
enabled = true
icmp_allow_router_advertisement = yes
icmp_allow_neighbor_advertisement = yes

[ACCEPTANCE]
enabled = yes
duration = 1
timestamp =

[EXEC]
iptables = /sbin/iptables
ip6tables = /sbin/ip6tables
iptables-save = /sbin/iptables-save
ip6tables-save = /sbin/ip6tables-save
iptables-restore = /sbin/iptables-restore
ip6tables-restore = /sbin/ip6tables-restore

"""
        create_file_if_not_exists(self.config_file)
        write_into_file(self.config_file, content)
        self.cfg = Config(self.config_file)
        self.easywall = Easywall(self.cfg)
        self.easywall.rules.ensure_files_exist()

    def tearDown(self) -> None:
        delete_file_if_exists(self.config_file)

    def test_init(self) -> None:
        """
        TODO: Doku
        """
        self.easywall = Easywall(self.cfg)

    def test_apply_not_accepted(self) -> None:
        """
        TODO: Doku
        """
        self.easywall.apply()

    def test_apply_accepted(self) -> None:
        """
        TODO: Doku
        """
        write_into_file(self.easywall.acceptance.filename, "true")
        self.easywall.apply()

    def test_apply_blacklist(self) -> None:
        """
        TODO: Doku
        """
        write_into_file(
            "{}/current/blacklist".format(self.easywall.rules.rulesfolder),
            """192.168.233.254
1.2.4.5
2001:db8:a0b:12f0::1
""")
        self.easywall.apply_blacklist()

    def test_apply_whitelist(self) -> None:
        """
        TODO: Doku
        """
        write_into_file(
            "{}/current/whitelist".format(self.easywall.rules.rulesfolder),
            """192.168.233.254
1.2.4.5
2001:db8:a0b:12f0::1
""")
        self.easywall.apply_whitelist()

    def test_apply_rules_port_range(self) -> None:
        """
        TODO: Doku
        """
        write_into_file("{}/current/udp".format(self.easywall.rules.rulesfolder), "1234:1237")
        self.easywall.apply_rules("udp")

    def test_apply_custom_rules(self) -> None:
        """
        TODO: Doku
        """
        write_into_file("{}/current/custom".format(self.easywall.rules.rulesfolder), "1234")
        self.easywall.apply_custom_rules()

    def test_apply_ssh_port(self) -> None:
        """
        TODO: Doku
        """
        write_into_file("{}/current/tcp".format(self.easywall.rules.rulesfolder), "22#ssh")
        self.easywall.apply_rules("tcp")

    def test_apply_forwarding(self) -> None:
        """
        TODO: Doku
        """
        write_into_file(
            "{}/current/forwarding".format(self.easywall.rules.rulesfolder),
            "tcp:1234:1235")
        self.easywall.apply_forwarding()
