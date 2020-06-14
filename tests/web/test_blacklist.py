"""
TODO: Doku
"""
from tests import unittest
from tests.web.test_login import TestLogin
from tests.web.utils import (prepare_client, prepare_configuration,
                             restore_configuration)


class TestBlacklist(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self):
        self.client = prepare_client()
        prepare_configuration()
        self.login = TestLogin()

    def tearDown(self):
        restore_configuration()

    def test_blacklist(self):
        """
        TODO: Doku
        """
        self.login.log_in(self.client)
        self.client.get('/blacklist')
