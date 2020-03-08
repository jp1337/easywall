"""
this file is the test module for the utility module
"""
import os
from datetime import datetime, timedelta
from time import sleep
from easywall.utility import (create_file_if_not_exists,
                              create_folder_if_not_exists, csv_to_array,
                              delete_file_if_exists, execute_os_command,
                              file_exists, file_get_contents,
                              get_abs_path_of_filepath, is_float, is_int,
                              rename_file, time_duration_diff, urlencode,
                              write_into_file, folder_exists)
from tests import unittest


class TestUtility(unittest.TestCase):
    """
    this class contains all test functions for the utility module
    """

    def test_folder(self):
        """
        TODO: Doku
        """
        foldername = "pytestfolder"
        create_folder_if_not_exists(foldername)
        sleep(1)
        self.assertTrue(folder_exists(foldername))
        sleep(1)
        os.removedirs(foldername)
        sleep(1)
        self.assertFalse(folder_exists(foldername))

    def test_file(self):
        """
        TODO: Doku
        """
        assert not file_exists("testfile")
        create_file_if_not_exists("testfile")
        assert file_exists("testfile")
        write_into_file("testfile", "testcontent")
        assert file_get_contents("testfile") == "testcontent"
        self.assertGreater(len(get_abs_path_of_filepath("testfile")), 0)
        rename_file("testfile", "testfilenew")
        assert not file_exists("testfile")
        assert file_exists("testfilenew")
        delete_file_if_exists("testfilenew")
        assert not file_exists("testfile")
        assert not file_exists("testfilenew")

    def test_float_int(self):
        """
        TODO: Doku
        """
        assert is_float(1)
        assert is_float(1.1)
        assert is_float(1.0)
        assert not is_float("test")
        assert is_int(1)
        assert is_int(1.0)
        assert not is_int(1.1)
        assert not is_int("test")

    def test_csv(self):
        """
        TODO: Doku
        """
        self.assertListEqual(csv_to_array("var1;var2", ";"), ["var1", "var2"])

    def test_urlencode(self):
        """
        TODO: Doku
        """
        self.assertEqual(urlencode("asd asd +"), "asd%20asd%20%2B")

    def test_time_duration_diff(self):
        """
        TODO: Doku
        """
        date1 = datetime.now()
        date2 = date1 + timedelta(0, 5)
        self.assertEqual(time_duration_diff(date1, date2), "5 seconds")
        date1 = datetime.now()
        date2 = datetime.now()
        self.assertEqual(time_duration_diff(date1, date2), "1 second")

    def test_execute_os_command(self):
        """
        TODO: Doku
        """
        self.assertTrue(execute_os_command("touch testfile"))
        self.assertTrue(file_exists("testfile"))
        delete_file_if_exists("testfile")

    def test_execute_os_command_error(self):
        """
        TODO: Doku
        """
        self.assertFalse(execute_os_command("/bin/false"))
