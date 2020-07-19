"""
TODO: Doku
"""
import hashlib
import platform
from typing import Any

from flask.testing import FlaskClient

from easywall.config import Config
from tests import unittest
from tests.web.utils import (CONFIG_PATH, prepare_client,
                             prepare_configuration, restore_configuration)


class TestLogin(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self) -> None:
        prepare_configuration()
        self.client = prepare_client()
        self.config = Config(CONFIG_PATH)

    def tearDown(self) -> None:
        restore_configuration()

    def test_login(self) -> None:
        """
        TODO: Doku
        """
        self.client.get('/login')

    def test_login_post(self) -> None:
        """
        TODO: Doku
        """
        self.log_in(self.client)

    def test_logout(self) -> None:
        """
        TODO: Doku
        """
        self.log_in(self.client)
        self.client.get('/logout', follow_redirects=True)

    def test_failed_log_in(self) -> Any:
        """
        TODO: Doku
        """
        self.set_username_password()
        return self.client.post('/login', data=dict(
            username="max",
            password="mustermann"
        ), follow_redirects=True)

    def set_username_password(self) -> None:
        """
        TODO: Doku
        """
        self.config = Config(CONFIG_PATH)
        self.config.set_value("WEB", "username", "test")
        hostname = platform.node().encode("utf-8")
        salt = hashlib.sha512(hostname).hexdigest()
        pw_hash = hashlib.sha512(
            str(salt + "test").encode("utf-8")).hexdigest()
        self.config.set_value("WEB", "password", pw_hash)

    def log_in(self, client: FlaskClient) -> Any:
        """
        TODO: Doku
        """
        self.set_username_password()
        return client.post('/login', data=dict(
            username="test",
            password="test"
        ), follow_redirects=True)
