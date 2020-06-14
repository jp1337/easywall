"""
TODO: Doku
"""
from os import urandom
from time import time

from easywall.config import Config
from easywall.utility import (create_file_if_not_exists, file_exists,
                              rename_file, write_into_file)
from easywall_web.__main__ import APP, CONFIG_PATH

CONFIG_BACKUP_PATH = "config/web.ini.backup"


def prepare_configuration():
    """
    TODO: Doku
    """

    if file_exists(CONFIG_PATH):
        rename_file(CONFIG_PATH, CONFIG_BACKUP_PATH)

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
version = 0.0.0
sha = 12345
date = 2020-01-01T00:00:00Z
timestamp = 1234

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
    config = Config(CONFIG_PATH)
    config.set_value("VERSION", "timestamp", str(int(time())))


def restore_configuration():
    """
    TODO: Doku
    """
    if file_exists(CONFIG_BACKUP_PATH):
        rename_file(CONFIG_BACKUP_PATH, CONFIG_PATH)


def prepare_client() -> object:
    """
    TODO: Doku
    """
    APP.config['TESTING'] = True
    APP.secret_key = urandom(12)
    with APP.test_client() as client:
        pass
    return client
