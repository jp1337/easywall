"""TODO: Doku."""
from tests import unittest

from easywall.rules_handler import RulesHandler


class TestRulesHandler(unittest.TestCase):
    """TODO: Doku."""

    def setUp(self) -> None:
        """TODO: Doku."""
        self.rules = RulesHandler()

    def test_get_current_rules(self) -> None:
        """TODO: Doku."""
        ports = ["80", "443"]
        self.rules.save_new_rules("tcp", ports)
        self.rules.apply_new_rules()
        self.assertEqual(self.rules.get_current_rules("tcp"), ["80", "443"])

    def test_get_new_rules(self) -> None:
        """TODO: Doku."""
        ports = ["80", "443"]
        self.rules.save_new_rules("tcp", ports)
        self.assertEqual(self.rules.get_new_rules("tcp"), ["80", "443"])

    def test_backup_current_rules(self) -> None:
        """TODO: Doku."""
        ports = ["80", "443"]
        self.rules.save_new_rules("tcp", ports)
        self.rules.apply_new_rules()
        self.rules.backup_current_rules()
        self.assertEqual(self.rules.get_backup_rules("tcp"), ["80", "443"])

    def test_apply_new_rules(self) -> None:
        """TODO: Doku."""
        ports = ["80", "443"]
        self.rules.save_new_rules("tcp", [])
        self.rules.apply_new_rules()
        self.assertEqual(self.rules.get_current_rules("tcp"), [])
        self.rules.save_new_rules("tcp", ports)
        self.rules.apply_new_rules()
        self.assertEqual(self.rules.get_current_rules("tcp"), ["80", "443"])

    def test_get_rules_for_web(self) -> None:
        """TODO: Doku."""
        ports = ["80", "443"]
        self.rules.save_new_rules("tcp", ports)
        self.rules.apply_new_rules()
        self.assertEqual(self.rules.get_rules_for_web("tcp"), ["80", "443"])
        ports = ["80", "443", "8080"]
        self.rules.save_new_rules("tcp", ports)
        self.assertEqual(self.rules.get_rules_for_web("tcp"), ["80", "443", "8080"])

    def test_rollback_from_backup(self) -> None:
        """TODO: Doku."""
        ports = ["80", "443"]
        self.rules.save_new_rules("tcp", ports)
        self.rules.apply_new_rules()
        self.rules.backup_current_rules()
        self.rules.save_new_rules("tcp", [])
        self.rules.apply_new_rules()
        self.assertEqual(self.rules.get_current_rules("tcp"), [])
        self.rules.rollback_from_backup()
        self.assertEqual(self.rules.get_current_rules("tcp"), ["80", "443"])

    def test_diff_new_current(self) -> None:
        """TODO: Doku."""
        self.rules.save_new_rules("tcp", ["123", "1234"])
        self.rules.apply_new_rules()
        self.rules.save_new_rules("tcp", ["1337"])
        self.assertTrue(self.rules.diff_new_current("tcp"))
        self.rules.apply_new_rules()
        self.assertFalse(self.rules.diff_new_current("tcp"))

    def test_save_new_rules(self) -> None:
        """TODO: Doku."""
        self.rules.save_new_rules("tcp", ["80", "443"])
        self.assertEqual(self.rules.get_new_rules("tcp"), ["80", "443"])
