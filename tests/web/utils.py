"""
TODO: Doku
"""
from os import urandom
from time import time

from flask.testing import FlaskClient

from easywall.config import Config
from easywall.utility import (create_file_if_not_exists, file_exists,
                              rename_file, write_into_file)

CONFIG_PATH = "config/web.ini"
CONFIG_BACKUP_PATH = "config/web.ini.backup"


def prepare_configuration() -> None:
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
login_attempts = 10
login_bantime = 1800

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
wsgi-file = easywall/web/__main__.py
need-plugin = python3
"""

    create_file_if_not_exists(CONFIG_PATH)
    write_into_file(CONFIG_PATH, content)
    config = Config(CONFIG_PATH)
    config.set_value("VERSION", "timestamp", str(int(time())))


def restore_configuration() -> bool:
    """
    TODO: Doku
    """
    if file_exists(CONFIG_BACKUP_PATH):
        rename_file(CONFIG_BACKUP_PATH, CONFIG_PATH)
        return True
    return False


def prepare_client() -> FlaskClient:
    """
    TODO: Doku
    """
    from easywall.web.__main__ import APP
    APP.config['TESTING'] = True
    APP.secret_key = urandom(12)
    return APP.test_client()
