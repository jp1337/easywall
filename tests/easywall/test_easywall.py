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

    def setUp(self):
        self.config_file = "test_easywall.ini"
        content = """[LOG]
level = info
to_files = false
to_stdout = true
filepath =
filename =

[IPV6]
enabled = true

[ACCEPTANCE]
enabled = true
duration = 1

[EXEC]
iptables = /sbin/iptables
ip6tables = /sbin/ip6tables
iptables-save = /sbin/iptables-save
ip6tables-save = /sbin/ip6tables-save
iptables-restore = /sbin/iptables-restore
ip6tables-restore = /sbin/ip6tables-restore

[BACKUP]
filepath = ./backup
ipv4filename = iptables_v4_backup
ipv6filename = iptables_v6_backup
        """
        create_file_if_not_exists(self.config_file)
        write_into_file(self.config_file, content)
        self.cfg = Config(self.config_file)
        self.easywall = Easywall(self.cfg)
        self.easywall.rules.rules_firstrun()

    def tearDown(self):
        delete_file_if_exists(self.config_file)

    def test_init(self):
        """
        TODO: Doku
        """
        self.easywall = Easywall(self.cfg)

    def test_apply_not_accepted(self):
        """
        TODO: Doku
        """
        self.easywall.apply()

    def test_apply_accepted(self):
        """
        TODO: Doku
        """
        write_into_file(self.easywall.acceptance.filename, "true")
        self.easywall.apply()

    def test_apply_blacklist(self):
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

    def test_apply_whitelist(self):
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

    def test_apply_rules_port_range(self):
        """
        TODO: Doku
        """
        write_into_file("{}/current/udp".format(self.easywall.rules.rulesfolder), "1234:1237")
        self.easywall.apply_rules("udp")

    def test_apply_custom_rules(self):
        """
        TODO: Doku
        """
        write_into_file("{}/current/custom".format(self.easywall.rules.rulesfolder), "1234")
        self.easywall.apply_custom_rules()
