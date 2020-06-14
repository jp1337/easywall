"""
TODO: Doku
"""
from easywall_web.__main__ import APP

from tests import unittest


class TestWhitelist(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self):
        APP.config['TESTING'] = True
        with APP.test_client() as self.client:
            pass

    def test_whitelist(self):
        """
        TODO: Doku
        """
        self.client.get('/whitelist')
