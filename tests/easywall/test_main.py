"""TODO: Doku."""
from tests import unittest
from tests.utils import prepare_configuration, restore_configuration

from easywall.__main__ import Main, ModifiedHandler
from easywall.utility import delete_file_if_exists, delete_folder_if_exists


class TestMain(unittest.TestCase):
    """TODO: Doku."""

    def setUp(self) -> None:
        """TODO: Doku."""
        prepare_configuration()
        delete_folder_if_exists("rules")

    def tearDown(self) -> None:
        """TODO: Doku."""
        delete_file_if_exists("test.log")
        delete_file_if_exists(".acceptance_status")
        restore_configuration()

    def test_init(self) -> None:
        """TODO: Doku."""
        Main()

    def test_start_observer(self) -> None:
        """TODO: Doku."""
        main = Main()
        main.stop_flag = True
        main.start_observer()

    def test_shutdown(self) -> None:
        """TODO: Doku."""
        main = Main()
        main.observer.start()
        main.shutdown()

    def test_apply(self) -> None:
        """TODO: Doku."""
        main = Main()
        main.apply(".apply")

    def test_modify_handler(self) -> None:
        """TODO: Doku."""
        main = Main()
        ModifiedHandler(main.apply)
