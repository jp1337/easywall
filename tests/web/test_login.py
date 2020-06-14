"""
TODO: Doku
"""
from unittest.mock import patch

from easywall.config import Config
from easywall_web.passwd import Passwd

from tests import unittest
from tests.web.utils import (CONFIG_PATH, prepare_client,
                             prepare_configuration, restore_configuration)


class TestLogin(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self):
        self.client = prepare_client()
        prepare_configuration()

    def tearDown(self):
        restore_configuration()

    def test_login(self):
        """
        TODO: Doku
        """
        self.client.get('/login')

    def test_login_post(self):
        """
        TODO: Doku
        """
        self.log_in(self.client)

    def test_logout(self):
        """
        TODO: Doku
        """
        self.client.get('/logout')

    @patch("builtins.input")
    @patch("getpass.getpass")
    def set_username_password(self, input, getpass):
        """
        TODO: Doku
        """
        input.return_value = "test"
        getpass.return_value = "test"
        Passwd()

    def log_in(self, client):
        """
        TODO: Doku
        """
        self.config = Config(CONFIG_PATH)
        self.set_username_password()
        return client.post('/login', data=dict(
            username="test",
            password="test"
        ), follow_redirects=True)
