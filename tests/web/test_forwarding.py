"""TODO: Doku."""
from tests import unittest
from tests.utils import (prepare_client, prepare_configuration,
                         restore_configuration)
from tests.web.test_login import TestLogin


class TestForwarding(unittest.TestCase):
    """TODO: Doku."""

    def setUp(self) -> None:
        """TODO: Doku."""
        prepare_configuration()
        self.client = prepare_client()
        self.login = TestLogin()

    def tearDown(self) -> None:
        """TODO: Doku."""
        restore_configuration()

    def test_forwarding_logged_out(self) -> None:
        """TODO: Doku."""
        response = self.client.get('/forwarding')
        self.assertIn(b"Please log in", response.data)

    def test_forwarding_logged_in(self) -> None:
        """TODO: Doku."""
        self.login.log_in(self.client)
        response = self.client.get('/forwarding')
        self.assertIn(b"Forwarding", response.data)

    def test_forwarding_save_logged_out(self) -> None:
        """TODO: Doku."""
        response = self.client.post('/forwarding-save')
        self.assertIn(b"Please log in", response.data)

    def test_forwarding_save_logged_in_add(self) -> None:
        """TODO: Doku."""
        self.login.log_in(self.client)
        response = self.client.post('/forwarding-save', data={
            "tcpudp": "tcp",
            "source-port": "123",
            "destination-port": "1234"
        }, follow_redirects=True)
        self.assertIn(b"The Configuration was saved successfully", response.data)

    def test_forwarding_save_logged_in_remove(self) -> None:
        """TODO: Doku."""
        self.login.log_in(self.client)
        response = self.client.post('/forwarding-save', data={
            "remove": None,
            "tcp:1234:12345": None
        }, follow_redirects=True)
        self.assertIn(b"The Configuration was saved successfully", response.data)
