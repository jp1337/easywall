from easywall.config import Config
from easywall.utility import create_file_if_not_exists, write_into_file, delete_file_if_exists
from tests import *


class TestConfig(unittest.TestCase):

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
