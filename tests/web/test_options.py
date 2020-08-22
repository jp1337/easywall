"""TODO: Doku."""
from tests import unittest
from tests.utils import (prepare_client, prepare_configuration,
                         restore_configuration)
from tests.web.test_login import TestLogin


class TestOptions(unittest.TestCase):
    """TODO: Doku."""

    def setUp(self) -> None:
        """TODO: Doku."""
        prepare_configuration()
        self.client = prepare_client()
        self.login = TestLogin()

    def tearDown(self) -> None:
        """TODO: Doku."""
        restore_configuration()

    def test_options_logged_out(self) -> None:
        """TODO: Doku."""
        response = self.client.get('/options')
        self.assertIn(b"Please log in", response.data)

    def test_options_logged_in(self) -> None:
        """TODO: Doku."""
        self.login.log_in(self.client)
        response = self.client.get('/options')
        self.assertIn(b"Options", response.data)

    def test_options_save_logged_out(self) -> None:
        """TODO: Doku."""
        response = self.client.post('/options-save')
        self.assertIn(b"Please log in", response.data)

    def test_options_save_logged_in(self) -> None:
        """TODO: Doku."""
        self.login.log_in(self.client)
        response = self.client.post('/options-save', data=dict(
            section="VERSION",
            cfgtype="web",
            version="0.0.0"
        ), follow_redirects=True)
        self.assertIn(b"The Configuration was saved successfully", response.data)

    def test_options_save_logged_in_checkbox(self) -> None:
        """TODO: Doku."""
        self.login.log_in(self.client)
        response = self.client.post('/options-save', data=dict(
            section="LOG",
            cfgtype="web",
            checkbox_to_stdout="on"
        ), follow_redirects=True)
        self.assertIn(b"The Configuration was saved successfully", response.data)
