"""
TODO: Doku
"""
from typing import Any
from unittest.mock import patch

from easywall.config import Config
from tests import unittest
from tests.web.utils import (CONFIG_PATH, prepare_configuration,
                             restore_configuration)


class TestPasswd(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self) -> None:
        prepare_configuration()

    def tearDown(self) -> None:
        restore_configuration()

    @patch("builtins.input")
    @patch("getpass.getpass")
    def test_init(self, input: Any, getpass: Any) -> None:
        """
        TODO: Doku
        """
        input.return_value = "test"
        getpass.return_value = "test"
        from easywall_web.passwd import Passwd
        Passwd()
        self.config = Config(CONFIG_PATH)
        self.assertEqual(self.config.get_value("WEB", "username"), "test")
