"""TODO: Doku."""
from tests import unittest
from tests.utils import prepare_configuration, restore_configuration

from easywall.__main__ import CONFIG_PATH
from easywall.config import Config
from easywall.easywall import Easywall
from easywall.utility import write_into_file


class TestRulesHandler(unittest.TestCase):
    """TODO: Doku."""

    def setUp(self) -> None:
        """TODO: Doku."""
        prepare_configuration()
        self.cfg = Config(CONFIG_PATH)
        self.easywall = Easywall(self.cfg)
        self.easywall.rules.ensure_files_exist()

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
        write_into_file(
            "{}/current/blacklist".format(self.easywall.rules.rulesfolder),
            """192.168.233.254
1.2.4.5
2001:db8:a0b:12f0::1
""")
        self.easywall.apply_blacklist()

    def test_apply_whitelist(self) -> None:
        """TODO: Doku."""
        write_into_file(
            "{}/current/whitelist".format(self.easywall.rules.rulesfolder),
            """192.168.233.254
1.2.4.5
2001:db8:a0b:12f0::1
""")
        self.easywall.apply_whitelist()

    def test_apply_rules_port_range(self) -> None:
        """TODO: Doku."""
        write_into_file("{}/current/udp".format(self.easywall.rules.rulesfolder), "1234:1237")
        self.easywall.apply_rules("udp")

    def test_apply_custom_rules(self) -> None:
        """TODO: Doku."""
        write_into_file("{}/current/custom".format(self.easywall.rules.rulesfolder), "1234")
        self.easywall.apply_custom_rules()

    def test_apply_ssh_port(self) -> None:
        """TODO: Doku."""
        write_into_file("{}/current/tcp".format(self.easywall.rules.rulesfolder), "22#ssh")
        self.easywall.apply_rules("tcp")

    def test_apply_forwarding(self) -> None:
        """TODO: Doku."""
        write_into_file(
            "{}/current/forwarding".format(self.easywall.rules.rulesfolder),
            "tcp:1234:1235")
        self.easywall.apply_forwarding()
