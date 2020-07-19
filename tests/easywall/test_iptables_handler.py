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

    def setUp(self) -> None:
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

    def tearDown(self) -> None:
        self.iptables.reset()
        delete_file_if_exists("iptables.ini")

    def test_policy(self) -> None:
        """
        TODO: Doku
        """
        self.iptables.add_policy(Chain.FORWARD, Target.ACCEPT)

    def test_chain(self) -> None:
        """
        TODO: Doku
        """
        self.iptables.add_chain("PORTSCAN")
        self.iptables.delete_chain("PORTSCAN")

    def test_append(self) -> None:
        """
        TODO: Doku
        """
        self.iptables.add_chain("PORTSCAN")
        self.iptables.add_append(Chain.PORTSCAN, "-i lo -j ACCEPT")
        self.iptables.flush("PORTSCAN")

    def test_status(self) -> None:
        """
        TODO: Doku
        """
        self.iptables.status()

    def test_save(self) -> None:
        """
        TODO: Doku
        """
        self.iptables.save()

    def test_restore(self) -> None:
        """
        TODO: Doku
        """
        self.iptables.restore()
