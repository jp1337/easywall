"""TODO: Doku."""
from tests import unittest
from tests.utils import (prepare_client, prepare_configuration,
                         restore_configuration)
from tests.web.test_login import TestLogin


class TestCustom(unittest.TestCase):
    """TODO: Doku."""

    def setUp(self) -> None:
        """TODO: Doku."""
        prepare_configuration()
        self.client = prepare_client()
        self.login = TestLogin()

    def tearDown(self) -> None:
        """TODO: Doku."""
        restore_configuration()

    def test_custom_logged_out(self) -> None:
        """TODO: Doku."""
        response = self.client.get('/custom')
        self.assertIn(b"Please log in", response.data)

    def test_custom_logged_in(self) -> None:
        """TODO: Doku."""
        self.login.log_in(self.client)
        response = self.client.get('/custom')
        self.assertIn(b"Custom", response.data)

    def test_custom_save_logged_out(self) -> None:
        """TODO: Doku."""
        response = self.client.post('/custom-save')
        self.assertIn(b"Please log in", response.data)

    def test_custom_save_logged_in(self) -> None:
        """TODO: Doku."""
        self.login.log_in(self.client)
        response = self.client.post('/custom-save', data=dict(
            test="test"
        ), follow_redirects=True)
        self.assertIn(b"The Configuration was saved successfully", response.data)
