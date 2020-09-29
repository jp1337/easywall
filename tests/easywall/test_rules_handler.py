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
        ports: list = []
        entry: dict = {}
        entry["description"] = "test"
        entry["port"] = "80"
        entry["ssh"] = False
        ports.append(entry)
        entry = {}
        entry["description"] = "test"
        entry["port"] = "443"
        entry["ssh"] = False
        ports.append(entry)
        self.rules.save_new_rules("tcp", ports)
        self.rules.apply_new_rules()
        self.assertEqual(self.rules.get_current_rules("tcp"), ports)

    def test_get_new_rules(self) -> None:
        """TODO: Doku."""
        ports: list = []
        entry: dict = {}
        entry["description"] = "test"
        entry["port"] = "80"
        entry["ssh"] = False
        ports.append(entry)
        entry = {}
        entry["description"] = "test"
        entry["port"] = "443"
        entry["ssh"] = False
        ports.append(entry)
        self.rules.save_new_rules("tcp", ports)
        self.assertEqual(self.rules.get_new_rules("tcp"), ports)

    def test_backup_current_rules(self) -> None:
        """TODO: Doku."""
        ports: list = []
        entry: dict = {}
        entry["description"] = "test"
        entry["port"] = "80"
        entry["ssh"] = False
        ports.append(entry)
        entry = {}
        entry["description"] = "test"
        entry["port"] = "443"
        entry["ssh"] = False
        ports.append(entry)
        self.rules.save_new_rules("tcp", ports)
        self.rules.apply_new_rules()
        self.rules.backup_current_rules()
        self.assertEqual(self.rules.get_backup_rules("tcp"), ports)

    def test_apply_new_rules(self) -> None:
        """TODO: Doku."""
        ports: list = []
        entry: dict = {}
        entry["description"] = "test"
        entry["port"] = "80"
        entry["ssh"] = False
        ports.append(entry)
        entry = {}
        entry["description"] = "test"
        entry["port"] = "443"
        entry["ssh"] = False
        ports.append(entry)
        self.rules.save_new_rules("tcp", [])
        self.rules.apply_new_rules()
        self.assertEqual(self.rules.get_current_rules("tcp"), [])
        self.rules.save_new_rules("tcp", ports)
        self.rules.apply_new_rules()
        self.assertEqual(self.rules.get_current_rules("tcp"), ports)

    def test_get_rules_for_web(self) -> None:
        """TODO: Doku."""
        ports: list = []
        entry: dict = {}
        entry["description"] = "test"
        entry["port"] = "80"
        entry["ssh"] = False
        ports.append(entry)
        entry = {}
        entry["description"] = "test"
        entry["port"] = "443"
        entry["ssh"] = False
        ports.append(entry)
        self.rules.save_new_rules("tcp", ports)
        self.rules.apply_new_rules()
        self.assertEqual(self.rules.get_rules_for_web("tcp"), ports)
        ports = []
        entry = {}
        entry["description"] = "test"
        entry["port"] = "80"
        entry["ssh"] = False
        ports.append(entry)
        entry = {}
        entry["description"] = "test"
        entry["port"] = "443"
        entry["ssh"] = False
        ports.append(entry)
        entry = {}
        entry["description"] = "test"
        entry["port"] = "8080"
        entry["ssh"] = False
        ports.append(entry)
        self.rules.save_new_rules("tcp", ports)
        self.assertEqual(self.rules.get_rules_for_web("tcp"), ports)

    def test_rollback_from_backup(self) -> None:
        """TODO: Doku."""
        ports: list = []
        entry: dict = {}
        entry["description"] = "test"
        entry["port"] = "80"
        entry["ssh"] = False
        ports.append(entry)
        entry = {}
        entry["description"] = "test"
        entry["port"] = "443"
        entry["ssh"] = False
        ports.append(entry)
        self.rules.save_new_rules("tcp", ports)
        self.rules.apply_new_rules()
        self.rules.backup_current_rules()
        self.rules.save_new_rules("tcp", [])
        self.rules.apply_new_rules()
        self.assertEqual(self.rules.get_current_rules("tcp"), [])
        self.rules.rollback_from_backup()
        self.assertEqual(self.rules.get_current_rules("tcp"), ports)

    def test_diff_new_current(self) -> None:
        """TODO: Doku."""
        ports: list = []
        entry: dict = {}
        entry["description"] = "test"
        entry["port"] = "123"
        entry["ssh"] = False
        ports.append(entry)
        entry = {}
        entry["description"] = "test"
        entry["port"] = "1234"
        entry["ssh"] = False
        ports.append(entry)
        self.rules.save_new_rules("tcp", ports)
        self.rules.apply_new_rules()
        ports = []
        entry = {}
        entry["description"] = "test"
        entry["port"] = "1337"
        entry["ssh"] = False
        ports.append(entry)
        self.rules.save_new_rules("tcp", ports)
        self.assertTrue(self.rules.diff_new_current("tcp"))
        self.rules.apply_new_rules()
        self.assertFalse(self.rules.diff_new_current("tcp"))

    def test_save_new_rules(self) -> None:
        """TODO: Doku."""
        ports: list = []
        entry: dict = {}
        entry["description"] = "test"
        entry["port"] = "80"
        entry["ssh"] = False
        ports.append(entry)
        entry = {}
        entry["description"] = "test"
        entry["port"] = "443"
        entry["ssh"] = False
        ports.append(entry)
        self.rules.save_new_rules("tcp", ports)
        self.assertEqual(self.rules.get_new_rules("tcp"), ports)
