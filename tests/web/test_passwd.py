"""
TODO: Doku
"""
from unittest.mock import patch

from easywall.config import Config
from easywall_web.__main__ import CONFIG_PATH
from easywall_web.passwd import Passwd

from tests import unittest
from tests.web.utils import prepare_configuration, restore_configuration


class TestPasswd(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self):
        prepare_configuration()

    def tearDown(self):
        restore_configuration()

    @patch("builtins.input")
    @patch("getpass.getpass")
    def test_init(self, input, getpass):
        """
        TODO: Doku
        """
        input.return_value = "test"
        getpass.return_value = "test"
        Passwd()
        self.config = Config(CONFIG_PATH)
        self.assertEqual(self.config.get_value("WEB", "username"), "test")
