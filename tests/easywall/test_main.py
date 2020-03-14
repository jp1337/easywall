"""
TODO: Doku
"""
from easywall.__main__ import CONFIG_PATH, Main, ModifiedHandler
from easywall.utility import (create_file_if_not_exists,
                              delete_folder_if_exists, file_exists,
                              rename_file, write_into_file)

from tests import unittest


class TestMain(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self):
        self.config_backup_path = "config/easywall.ini.backup"
        if file_exists(CONFIG_PATH):
            rename_file(CONFIG_PATH, self.config_backup_path)

        content = """[LOG]
level = info
to_files = false
to_stdout = true
filepath =
filename =

[IPV6]
enabled = true

[ACCEPTANCE]
enabled = false
duration = 120
timestamp =

[EXEC]
iptables = /sbin/iptables
ip6tables = /sbin/ip6tables
iptables-save = /sbin/iptables-save
ip6tables-save = /sbin/ip6tables-save
iptables-restore = /sbin/iptables-restore
ip6tables-restore = /sbin/ip6tables-restore

[BACKUP]
filepath = ./backup
ipv4filename = iptables_v4_backup
ipv6filename = iptables_v6_backup
        """
        create_file_if_not_exists(CONFIG_PATH)
        write_into_file(CONFIG_PATH, content)
        delete_folder_if_exists("rules")

    def tearDown(self):
        if file_exists(self.config_backup_path):
            rename_file(self.config_backup_path, CONFIG_PATH)

    def test_init(self):
        """
        TODO: Doku
        """
        Main()

    def test_start_observer(self):
        """
        TODO: Doku
        """
        main = Main()
        main.stop_flag = True
        main.start_observer()

    def test_shutdown(self):
        """
        TODO: Doku
        """
        main = Main()
        main.observer.start()
        main.shutdown()

    def test_apply(self):
        """
        TODO: Doku
        """
        main = Main()
        main.apply(".apply")

    def test_modify_handler(self):
        """
        TODO: Doku
        """
        main = Main()
        ModifiedHandler(main.apply)
