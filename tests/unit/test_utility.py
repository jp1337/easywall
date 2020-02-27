"""
this file is the test module for the utility module
"""
import os

from easywall.utility import (create_file_if_not_exists,
                              create_folder_if_not_exists,
                              delete_file_if_exists,
                              write_into_file,
                              file_get_contents,
                              get_abs_path_of_filepath,
                              rename_file,
                              file_exists,
                              is_float,
                              is_int,
                              csv_to_array,
                              urlencode,
                              time_duration_diff,
                              execute_os_command)
from datetime import datetime, timedelta
from tests import unittest


class TestUtility(unittest.TestCase):
    """
    this class contains all test functions for the utility module
    """

    def test_folder(self):
        create_folder_if_not_exists("testfolder")
        assert os.path.exists("testfolder") == 1
        os.removedirs("testfolder")
        assert os.path.exists("testfolder") == 0

    def test_file(self):
        assert not file_exists("testfile")
        create_file_if_not_exists("testfile")
        assert file_exists("testfile")
        write_into_file("testfile", "testcontent")
        assert file_get_contents("testfile") == "testcontent"
        assert len(get_abs_path_of_filepath("testfile")) > 0
        rename_file("testfile", "testfilenew")
        assert not file_exists("testfile")
        assert file_exists("testfilenew")
        delete_file_if_exists("testfilenew")
        assert not file_exists("testfile")
        assert not file_exists("testfilenew")

    def test_float_int(self):
        assert is_float(1)
        assert is_float(1.1)
        assert is_float(1.0)
        assert not is_float("test")
        assert is_int(1)
        assert is_int(1.0)
        assert not is_int(1.1)
        assert not is_int("test")

    def test_csv(self):
        self.assertListEqual(csv_to_array("var1;var2", ";"), ["var1", "var2"])

    def test_urlencode(self):
        self.assertEqual(urlencode("asd asd +"), "asd%20asd%20%2B")

    def test_time_duration_diff(self):
        date1 = datetime.now()
        date2 = date1 + timedelta(0, 5)
        self.assertEqual(time_duration_diff(date1, date2), "5 seconds")

    def test_execute_os_command(self):
        self.assertTrue(execute_os_command("touch testfile"))
        self.assertTrue(file_exists("testfile"))
        delete_file_if_exists("testfile")

    def test_execute_os_command_error(self):
        self.assertFalse(execute_os_command("/bin/false"))
