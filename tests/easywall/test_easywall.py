"""TODO: Doku."""
from tests import unittest
from tests.utils import prepare_configuration, restore_configuration

from easywall.__main__ import CONFIG_PATH
from easywall.config import Config
from easywall.easywall import Easywall
from easywall.rules_handler import RulesHandler
from easywall.utility import write_into_file


class TestRulesHandler(unittest.TestCase):
    """TODO: Doku."""

    def setUp(self) -> None:
        """TODO: Doku."""
        prepare_configuration()
        self.cfg = Config(CONFIG_PATH)
        self.easywall = Easywall(self.cfg)
        self.rules = RulesHandler()

    def tearDown(self) -> None:
        """TODO: Doku."""
        restore_configuration()

    def test_init(self) -> None:
        """TODO: Doku."""
        self.easywall = Easywall(self.cfg)

    def test_apply_not_accepted(self) -> None:
        """TODO: Doku."""
        self.easywall.apply()

    def test_apply_accepted(self) -> None:
        """TODO: Doku."""
        write_into_file(self.easywall.acceptance.filename, "true")
        self.easywall.apply()

    def test_apply_blacklist(self) -> None:
        """TODO: Doku."""
        blacklist = ["192.168.233.254", "1.2.4.5", "2001:db8:a0b:12f0::1"]
        self.rules.save_new_rules("blacklist", blacklist)
        self.rules.apply_new_rules()
        self.easywall.apply_blacklist()

    def test_apply_whitelist(self) -> None:
        """TODO: Doku."""
        whitelist = ["192.168.233.254", "1.2.4.5", "2001:db8:a0b:12f0::1"]
        self.rules.save_new_rules("whitelist", whitelist)
        self.rules.apply_new_rules()
        self.easywall.apply_whitelist()

    def test_apply_rules_port_range(self) -> None:
        """TODO: Doku."""
        udp = ["1234:1237"]
        self.rules.save_new_rules("udp", udp)
        self.rules.apply_new_rules()
        self.easywall.apply_rules("udp")

    def test_apply_custom_rules(self) -> None:
        """TODO: Doku."""
        custom = ["1234"]
        self.rules.save_new_rules("custom", custom)
        self.rules.apply_new_rules()
        self.easywall.apply_custom_rules()

    def test_apply_ssh_port(self) -> None:
        """TODO: Doku."""
        tcp = ["22#ssh"]
        self.rules.save_new_rules("tcp", tcp)
        self.rules.apply_new_rules()
        self.easywall.apply_rules("tcp")

    def test_apply_forwarding(self) -> None:
        """TODO: Doku."""
        forwarding = ["tcp:1234:1235"]
        self.rules.save_new_rules("forwarding", forwarding)
        self.rules.apply_new_rules()
        self.easywall.apply_forwarding()
