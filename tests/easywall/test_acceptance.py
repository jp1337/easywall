"""TODO: Doku."""
from tests import unittest
from tests.utils import prepare_configuration, restore_configuration

from easywall.__main__ import CONFIG_PATH
from easywall.acceptance import Acceptance
from easywall.config import Config
from easywall.utility import write_into_file


class TestAcceptance(unittest.TestCase):
    """TODO: Doku."""

    def setUp(self) -> None:
        """TODO: Doku."""
        prepare_configuration()
        self.config = Config(CONFIG_PATH)
        self.acceptance = Acceptance(self.config)

    def tearDown(self) -> None:
        """TODO: Doku."""
        restore_configuration()

    def test_disabled(self) -> None:
        """TODO: Doku."""
        self.config = Config(CONFIG_PATH)
        self.config.set_value("ACCEPTANCE", "enabled", "no")
        self.acceptance = Acceptance(self.config)
        self.assertEqual(self.acceptance.status(), "disabled")

    def test_not_accepted(self) -> None:
        """TODO: Doku."""
        self.acceptance.start()
        self.acceptance.wait()
        self.assertEqual(self.acceptance.status(), "not accepted")

    def test_accepted(self) -> None:
        """TODO: Doku."""
        self.acceptance.start()
        self.acceptance.wait()
        write_into_file(self.acceptance.filename, "true")
        self.assertEqual(self.acceptance.status(), "accepted")

    def test_accepted_early(self) -> None:
        """TODO: Doku."""
        self.acceptance.start()
        write_into_file(self.acceptance.filename, "true")
        self.acceptance.wait()
        self.assertEqual(self.acceptance.status(), "accepted")
