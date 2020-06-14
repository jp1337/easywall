"""
TODO: Doku
"""
from easywall.config import Config
from easywall.utility import (create_file_if_not_exists, file_exists,
                              rename_file, write_into_file)
from easywall_web.__main__ import APP, CONFIG_PATH

from tests import unittest


class TestBlacklist(unittest.TestCase):
    """
    TODO: Doku
    """

    def setUp(self):
        self.config_backup_path = "config/web.ini.backup"
        if file_exists(CONFIG_PATH):
            rename_file(CONFIG_PATH, self.config_backup_path)

        content = """[LOG]
level = info
to_files = no
to_stdout = yes
filepath = log
filename = easywall-web.log

[WEB]
username = demo
password = xxx
bindip = 0.0.0.0
bindport = 12227

[VERSION]
version = 0.0.4
sha = 30575157577b53d2f8168d9ffe588df2de8f1dd6
date = 2020-03-14T23:23:58Z
timestamp = 1591549571

[uwsgi]
https-socket = 0.0.0.0:12227,easywall.crt,easywall.key
processes = 5
threads = 2
callable = APP
master = false
wsgi-file = easywall_web/__main__.py
need-plugin = python3
"""
        create_file_if_not_exists(CONFIG_PATH)
        write_into_file(CONFIG_PATH, content)
        self.config = Config(CONFIG_PATH)

        APP.config['TESTING'] = True
        with APP.test_client() as self.client:
            pass

    def tearDown(self):
        if file_exists(self.config_backup_path):
            rename_file(self.config_backup_path, CONFIG_PATH)

    def test_blacklist(self):
        """
        TODO: Doku
        """
        self.client.get('/blacklist')
