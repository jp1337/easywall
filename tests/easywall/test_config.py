"""TODO: Doku."""
from configparser import ParsingError

from tests import unittest

from easywall.config import Config
from easywall.utility import (create_file_if_not_exists, delete_file_if_exists,
                              write_into_file)


class TestConfig(unittest.TestCase):
    """TODO: Doku."""

    def setUp(self) -> None:
        """TODO: Doku."""
        content = """[TEST]
        teststring = string
        testboolean = true
        testint = 1
        testfloat = 1.1
        """
        create_file_if_not_exists("test.ini")
        write_into_file("test.ini", content)

        self.config = Config("test.ini")

    def tearDown(self) -> None:
        """TODO: Doku."""
        delete_file_if_exists("test.ini")

    def test_constructor_file_not_found(self) -> None:
        """TODO: Doku."""
        with self.assertRaises(FileNotFoundError):
            Config("test2.ini")

    def test_constructor_file_not_read(self) -> None:
        """TODO: Doku."""
        create_file_if_not_exists("test.ini")
        content = """[DEFAULT]
        goodcontent = test
        badcontent
        """
        write_into_file("test.ini", content)
        with self.assertRaises(ParsingError):
            Config("test.ini")

    def test_get_value_error(self) -> None:
        """TODO: Doku."""
        self.assertEqual(self.config.get_value("TEST", "notexistent"), "")

    def test_get_value_bool(self) -> None:
        """TODO: Doku."""
        self.assertEqual(self.config.get_value("TEST", "testboolean"), True)

    def test_get_value_int(self) -> None:
        """TODO: Doku."""
        self.assertEqual(self.config.get_value("TEST", "testint"), 1)

    def test_get_value_float(self) -> None:
        """TODO: Doku."""
        self.assertEqual(self.config.get_value(
            "TEST", "testfloat"), float(1.1))

    def test_get_sections(self) -> None:
        """TODO: Doku."""
        self.assertIn("TEST", self.config.get_sections())

    def test_set_value_success(self) -> None:
        """TODO: Doku."""
        self.assertEqual(self.config.set_value(
            "TEST", "teststring", "erfolg"), True)

    def test_set_value_fail_section(self) -> None:
        """TODO: Doku."""
        self.assertEqual(self.config.set_value("TEST2", "asd", "asd"), False)
