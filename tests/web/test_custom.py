"""
TODO: Doku
"""
from easywall_web.__main__ import APP

from tests import unittest


class TestCustom(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self):
        APP.config['TESTING'] = True
        with APP.test_client() as self.client:
            pass

    def test_custom(self):
        """
        TODO: Doku
        """
        self.client.get('/custom')
