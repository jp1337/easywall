"""this file is the test module for the log module."""
from logging import CRITICAL, DEBUG, ERROR, INFO, NOTSET, WARNING

from easywall.log import Log
from easywall.utility import delete_file_if_exists
from tests import unittest


class TestLog(unittest.TestCase):
    """this class contains all test functions for the log module."""

    def setUp(self) -> None:
        """TODO: Doku."""
        delete_file_if_exists("./test.log")
        self.log = Log("INFO", True, True, ".", "test.log")

    def tearDown(self) -> None:
        """TODO: Doku."""
        self.log.close_logging()
        delete_file_if_exists("./test.log")

    def test_correct_level_debug(self) -> None:
        """TODO: Doku."""
        self.assertEqual(self.log.correct_level("DEBUG"), DEBUG)

    def test_correct_level_info(self) -> None:
        """TODO: Doku."""
        self.assertEqual(self.log.correct_level("INFO"), INFO)

    def test_correct_level_warning(self) -> None:
        """TODO: Doku."""
        self.assertEqual(self.log.correct_level("WARNING"), WARNING)

    def test_correct_level_error(self) -> None:
        """TODO: Doku."""
        self.assertEqual(self.log.correct_level("ERROR"), ERROR)

    def test_correct_level_critical(self) -> None:
        """TODO: Doku."""
        self.assertEqual(self.log.correct_level("CRITICAL"), CRITICAL)

    def test_correct_level_unknown(self) -> None:
        """TODO: Doku."""
        self.assertEqual(self.log.correct_level("SOMETHING"), NOTSET)
