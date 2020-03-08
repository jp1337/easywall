"""
TODO: Doku
"""
from easywall.config import Config
from easywall.iptables_handler import Iptables, Chain, Target
from easywall.utility import (create_file_if_not_exists, delete_file_if_exists,
                              write_into_file)
from tests import unittest


class TestIPTablesHandler(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self):
        content = """[EXEC]
iptables = /sbin/iptables
ip6tables = /sbin/ip6tables
iptables-save = /sbin/iptables-save
ip6tables-save = /sbin/ip6tables-save
iptables-restore = /sbin/iptables-restore
ip6tables-restore = /sbin/ip6tables-restore

[IPV6]
enabled = yes

[BACKUP]
filepath = ./backup
ipv4filename = iptables_v4_backup
ipv6filename = iptables_v6_backup
"""
        create_file_if_not_exists("iptables.ini")
        write_into_file("iptables.ini", content)

        self.config = Config("iptables.ini")
        self.iptables = Iptables(self.config)

    def tearDown(self):
        self.iptables.reset()
        delete_file_if_exists("iptables.ini")

    def test_policy(self):
        """
        TODO: Doku
        """
        self.iptables.add_policy(Chain.FORWARD, Target.ACCEPT)

    def test_chain(self):
        """
        TODO: Doku
        """
        self.iptables.add_chain("TESTCHAIN")
        self.iptables.delete_chain("TESTCHAIN")

    def test_append(self):
        """
        TODO: Doku
        """
        self.iptables.add_chain("TESTCHAIN")
        self.iptables.add_append("TESTCHAIN", "-i lo -j ACCEPT")
        self.iptables.flush("TESTCHAIN")

    def test_status(self):
        """
        TODO: Doku
        """
        self.iptables.status()

    def test_save(self):
        """
        TODO: Doku
        """
        self.iptables.save()

    def test_restore(self):
        """
        TODO: Doku
        """
        self.iptables.restore()
