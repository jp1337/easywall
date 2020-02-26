"""
this file is the test module for the config module
"""
from easywall.config import Config
from easywall.utility import create_file_if_not_exists, write_into_file, delete_file_if_exists
from tests import unittest


class TestConfig(unittest.TestCase):
    """
    this class contains all test functions for the config module
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

    def test_get_value_error(self):
        self.assertEqual(self.config.get_value("TEST", "notexistent"), "")

    def test_get_value_bool(self):
        self.assertEqual(self.config.get_value("TEST", "testboolean"), True)

    def test_get_value_int(self):
        self.assertEqual(self.config.get_value("TEST", "testint"), 1)

    def test_get_value_float(self):
        self.assertEqual(self.config.get_value(
            "TEST", "testfloat"), float(1.1))

    def test_get_sections(self):
        self.assertIn("TEST", self.config.get_sections())

    def test_set_value_success(self):
        self.assertEqual(self.config.set_value(
            "TEST", "teststring", "erfolg"), True)

    def test_set_value_fail_section(self):
        self.assertEqual(self.config.set_value("TEST2", "asd", "asd"), False)
