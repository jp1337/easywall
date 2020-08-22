"""TODO: Doku."""
from tests import unittest
from tests.utils import (prepare_client, prepare_configuration,
                         restore_configuration)


class TestMain(unittest.TestCase):
    """TODO: Doku."""

    def setUp(self) -> None:
        """TODO: Doku."""
        prepare_configuration()
        self.client = prepare_client()

    def tearDown(self) -> None:
        """TODO: Doku."""
        restore_configuration()

    def test_init(self) -> None:
        """TODO: Doku."""
        from easywall.web.__main__ import Main
        Main(debug=False)
