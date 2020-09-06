"""TODO: Doku."""
from os import urandom
from time import time
from flask.testing import FlaskClient

from easywall.config import Config
from easywall.utility import (create_file_if_not_exists, file_exists,
                              rename_file, write_into_file)

EASYWALL_CONFIG_PATH = "config/easywall.ini"
EASYWALL_CONFIG_BACKUP_PATH = "config/easywall.ini.backup"
WEB_CONFIG_PATH = "config/web.ini"
WEB_CONFIG_BACKUP_PATH = "config/web.ini.backup"
LOG_CONFIG_PATH = "config/log.ini"
LOG_CONFIG_BACKUP_PATH = "config/log.ini.backup"


def prepare_configuration() -> None:
    """TODO: Doku."""
    if file_exists(EASYWALL_CONFIG_PATH):
        rename_file(EASYWALL_CONFIG_PATH, EASYWALL_CONFIG_BACKUP_PATH)
    if file_exists(WEB_CONFIG_PATH):
        rename_file(WEB_CONFIG_PATH, WEB_CONFIG_BACKUP_PATH)
    if file_exists(LOG_CONFIG_PATH):
        rename_file(LOG_CONFIG_PATH, LOG_CONFIG_BACKUP_PATH)

    content = """[IPTABLES]
log_blocked_connections = yes
log_blocked_connections_log_limit = 60
log_blacklist_connections = yes
log_blacklist_connections_log_limit = 60
drop_broadcast_packets = yes
drop_multicast_packets = yes
drop_anycast_packets = yes
ssh_brute_force_prevention = yes
ssh_brute_force_prevention_log = yes
ssh_brute_force_prevention_connection_limit = 5
ssh_brute_force_prevention_log_limit = 60
icmp_flood_prevention = yes
icmp_flood_prevention_log = yes
icmp_flood_prevention_connection_limit = 5
icmp_flood_prevention_log_limit = 60
drop_invalid_packets = yes
drop_invalid_packets_log = yes
drop_invalid_packets_log_limit = 60
port_scan_prevention = yes
port_scan_prevention_log = yes
port_scan_prevention_log_limit = 60

[IPV6]
enabled = yes
icmp_allow_router_advertisement = yes
icmp_allow_neighbor_advertisement = yes

[ACCEPTANCE]
enabled = yes
duration = 1
timestamp =

[EXEC]
iptables = /sbin/iptables
ip6tables = /sbin/ip6tables
iptables-save = /sbin/iptables-save
ip6tables-save = /sbin/ip6tables-save
iptables-restore = /sbin/iptables-restore
ip6tables-restore = /sbin/ip6tables-restore
"""

    create_file_if_not_exists(EASYWALL_CONFIG_PATH)
    write_into_file(EASYWALL_CONFIG_PATH, content)

    content = """[WEB]
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
ssl-option = 268435456
https-socket = 0.0.0.0:12227,ssl/easywall.crt,ssl/easywall.key,HIGH
processes = 5
threads = 2
callable = APP
master = yes
die-on-term = yes
wsgi-file = easywall/web/__main__.py
need-plugin = python3
"""

    create_file_if_not_exists(WEB_CONFIG_PATH)
    write_into_file(WEB_CONFIG_PATH, content)
    config = Config(WEB_CONFIG_PATH)
    config.set_value("VERSION", "timestamp", str(int(time())))

    content = """[LOG]
level = info
to_files = no
to_stdout = yes
filepath = /var/log
filename = easywall.log
"""

    create_file_if_not_exists(LOG_CONFIG_PATH)
    write_into_file(LOG_CONFIG_PATH, content)


def restore_configuration() -> bool:
    """TODO: Doku."""
    rccode = False
    if file_exists(EASYWALL_CONFIG_BACKUP_PATH):
        rename_file(EASYWALL_CONFIG_BACKUP_PATH, EASYWALL_CONFIG_PATH)
        rccode = True
    if file_exists(WEB_CONFIG_BACKUP_PATH):
        rename_file(WEB_CONFIG_BACKUP_PATH, WEB_CONFIG_PATH)
        rccode = True
    if file_exists(LOG_CONFIG_BACKUP_PATH):
        rename_file(LOG_CONFIG_BACKUP_PATH, LOG_CONFIG_PATH)
        rccode = True
    return rccode


def prepare_client() -> FlaskClient:
    """TODO: Doku."""
    from easywall.web.__main__ import APP
    APP.config['TESTING'] = True
    APP.secret_key = urandom(12)
    return APP.test_client()
