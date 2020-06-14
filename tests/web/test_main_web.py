"""
TODO: Doku
"""
from tests import unittest
from tests.web.utils import (prepare_client, prepare_configuration,
                             restore_configuration)


class TestMain(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self):
        self.client = prepare_client()
        prepare_configuration()

    def tearDown(self):
        restore_configuration()

    def test_init(self):
        """
        TODO: Doku
        """
        from easywall_web.__main__ import Main
        Main(debug=False)
