"""TODO: Doku."""
from typing import Any
from unittest.mock import patch

from tests import unittest
from tests.utils import (WEB_CONFIG_PATH, prepare_configuration,
                         restore_configuration)

from easywall.config import Config


class TestPasswd(unittest.TestCase):
    """TODO: Doku."""

    def setUp(self) -> None:
        """TODO: Doku."""
        prepare_configuration()

    def tearDown(self) -> None:
        """TODO: Doku."""
        restore_configuration()

    @patch("builtins.input")
    @patch("getpass.getpass")
    def test_init(self, input: Any, getpass: Any) -> None:
        """TODO: Doku."""
        input.return_value = "test"
        getpass.return_value = "test"
        from easywall.web.passwd import Passwd
        Passwd()
        self.config = Config(WEB_CONFIG_PATH)
        self.assertEqual(self.config.get_value("WEB", "username"), "test")
