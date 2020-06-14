"""
TODO: Doku
"""
from tests import unittest
from tests.web.test_login import TestLogin
from tests.web.utils import (prepare_client, prepare_configuration,
                             restore_configuration)


class TestOptions(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self):
        prepare_configuration()
        self.client = prepare_client()
        self.login = TestLogin()

    def tearDown(self):
        restore_configuration()

    def test_options_logged_out(self):
        """
        TODO: Doku
        """
        response = self.client.get('/options')
        self.assertIn(b"Please sign in", response.data)

    def test_options_logged_in(self):
        """
        TODO: Doku
        """
        self.login.log_in(self.client)
        response = self.client.get('/options')
        self.assertIn(b"Options", response.data)

    def test_options_save_logged_out(self):
        """
        TODO: Doku
        """
        response = self.client.post('/options-save')
        self.assertIn(b"Please sign in", response.data)

    def test_options_save_logged_in(self):
        """
        TODO: Doku
        """
        self.login.log_in(self.client)
        response = self.client.post('/options-save', data=dict(
            section="VERSION",
            version="0.0.0"
        ), follow_redirects=True)
        self.assertIn(b"The Configuration was saved successfully", response.data)

    def test_options_save_logged_in_checkbox(self):
        """
        TODO: Doku
        """
        self.login.log_in(self.client)
        response = self.client.post('/options-save', data=dict(
            section="LOG",
            checkbox_to_stdout="on"
        ), follow_redirects=True)
        self.assertIn(b"The Configuration was saved successfully", response.data)
