"""
TODO: Doku
"""
from easywall.acceptance import Acceptance
from easywall.config import Config
from easywall.utility import (create_file_if_not_exists, delete_file_if_exists,
                              write_into_file)
from tests import unittest


class TestAcceptance(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self):
        content = """[ACCEPTANCE]
        enabled = true
        duration = 1
        """
        create_file_if_not_exists("acceptance.ini")
        write_into_file("acceptance.ini", content)

        self.config = Config("acceptance.ini")
        self.acceptance = Acceptance(self.config)

    def tearDown(self):
        delete_file_if_exists("acceptance.ini")

    def test_disabled(self):
        """
        TODO: Doku
        """
        content = """[ACCEPTANCE]
        enabled = false
        duration = 1
        """
        create_file_if_not_exists("acceptance.ini")
        write_into_file("acceptance.ini", content)
        self.config = Config("acceptance.ini")
        self.acceptance = Acceptance(self.config)
        self.assertEqual(self.acceptance.status(), "disabled")

    def test_not_accepted(self):
        """
        TODO: Doku
        """
        self.acceptance.start()
        self.acceptance.wait()
        self.assertEqual(self.acceptance.status(), "not accepted")

    def test_accepted(self):
        """
        TODO: Doku
        """
        self.acceptance.start()
        self.acceptance.wait()
        write_into_file(self.acceptance.filename, "true")
        self.assertEqual(self.acceptance.status(), "accepted")
