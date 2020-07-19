"""
TODO: Doku
"""
from tests import unittest
from tests.web.test_login import TestLogin
from tests.web.utils import (prepare_client, prepare_configuration,
                             restore_configuration)


class TestWhitelist(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self) -> None:
        prepare_configuration()
        self.client = prepare_client()
        self.login = TestLogin()

    def tearDown(self) -> None:
        restore_configuration()

    def test_whitelist_logged_out(self) -> None:
        """
        TODO: Doku
        """
        response = self.client.get('/whitelist')
        self.assertIn(b"Please log in", response.data)

    def test_whitelist_logged_in(self) -> None:
        """
        TODO: Doku
        """
        self.login.log_in(self.client)
        response = self.client.get('/whitelist')
        self.assertIn(b"Whitelist", response.data)

    def test_whitelist_save_logged_out(self) -> None:
        """
        TODO: Doku
        """
        response = self.client.post('/whitelist-save')
        self.assertIn(b"Please log in", response.data)

    def test_whitelist_save_logged_in_new(self) -> None:
        """
        TODO: Doku
        """
        self.login.log_in(self.client)
        response = self.client.post('/whitelist-save', data=dict(
            ipadr="abc"
        ), follow_redirects=True)
        self.assertIn(b"The Configuration was saved successfully", response.data)

    def test_whitelist_save_logged_in_remove(self) -> None:
        """
        TODO: Doku
        """
        self.login.log_in(self.client)
        response = self.client.post('/whitelist-save', data=dict(
            abc=""
        ), follow_redirects=True)
        self.assertIn(b"The Configuration was saved successfully", response.data)
