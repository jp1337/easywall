"""
TODO: Doku
"""
from configparser import ParsingError

from easywall.config import Config
from easywall.utility import (create_file_if_not_exists,
                              delete_file_if_exists, write_into_file)
from tests import unittest


class TestConfig(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self):
        content = """[TEST]
        teststring = string
        testboolean = true
        testint = 1
        testfloat = 1.1
        """
        create_file_if_not_exists("test.ini")
        write_into_file("test.ini", content)

        self.config = Config("test.ini")

    def tearDown(self):
        delete_file_if_exists("test.ini")

    def test_constructor_file_not_found(self):
        """
        TODO: Doku
        """
        with self.assertRaises(FileNotFoundError):
            Config("test2.ini")

    def test_constructor_file_not_read(self):
        """
        TODO: Doku
        """
        create_file_if_not_exists("test.ini")
        content = """[DEFAULT]
        goodcontent = test
        badcontent
        """
        write_into_file("test.ini", content)
        with self.assertRaises(ParsingError):
            Config("test.ini")

    def test_get_value_error(self):
        """
        TODO: Doku
        """
        self.assertEqual(self.config.get_value("TEST", "notexistent"), "")

    def test_get_value_bool(self):
        """
        TODO: Doku
        """
        self.assertEqual(self.config.get_value("TEST", "testboolean"), True)

    def test_get_value_int(self):
        """
        TODO: Doku
        """
        self.assertEqual(self.config.get_value("TEST", "testint"), 1)

    def test_get_value_float(self):
        """
        TODO: Doku
        """
        self.assertEqual(self.config.get_value(
            "TEST", "testfloat"), float(1.1))

    def test_get_sections(self):
        """
        TODO: Doku
        """
        self.assertIn("TEST", self.config.get_sections())

    def test_set_value_success(self):
        """
        TODO: Doku
        """
        self.assertEqual(self.config.set_value(
            "TEST", "teststring", "erfolg"), True)

    def test_set_value_fail_section(self):
        """
        TODO: Doku
        """
        self.assertEqual(self.config.set_value("TEST2", "asd", "asd"), False)
