"""
TODO: Doku
"""
from easywall_web.__main__ import APP

from tests import unittest
from tests.web.test_login import TestLogin


class TestBlacklist(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self):
        APP.config['TESTING'] = True
        with APP.test_client() as self.client:
            pass

    def test_blacklist(self):
        """
        TODO: Doku
        """
        login = TestLogin()
        login.setUp()
        login.log_in()
        login.client.get('/blacklist')
        login.tearDown()
