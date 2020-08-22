"""the module contains a class webutils which is called in the route modules"""
import json
import platform
import time
import urllib
from datetime import datetime, timezone
from typing import Any

from flask import Request, session

from easywall.__main__ import CONFIG_PATH, LOG_CONFIG_PATH
from easywall.config import Config
from easywall.utility import file_exists, file_get_contents, time_duration_diff
from easywall.web.defaultpayload import DefaultPayload


class Webutils(object):
    """the class is called in the route modules and contains non route-specific functions"""

    def __init__(self) -> None:
        self.cfg = Config("config/web.ini")
        self.cfg_easywall = Config(CONFIG_PATH)
        self.cfg_log = Config(LOG_CONFIG_PATH)

    def check_login(self, request: Request) -> bool:
        """the function checks if the user/session is logged in"""
        if not session.get('logged_in'):
            return False
        if request.remote_addr != session.get('ip_address'):
            return False
        return True

    # -------------------------
    # Payload Operations

    def get_default_payload(self, title: str, css: str = "easywall") -> DefaultPayload:
        """the function creates a object of information that are needed on every page"""
        payload = DefaultPayload()
        payload.year = datetime.today().year
        payload.title = title
        payload.customcss = css
        payload.machine = self.get_machine_infos()
        payload.latest_version = str(self.cfg.get_value("VERSION", "version"))
        payload.current_version = file_get_contents(".version")
        payload.commit_sha = str(self.cfg.get_value("VERSION", "sha"))
        payload.commit_date = self.get_commit_date(str(self.cfg.get_value("VERSION", "date")))
        payload.config_mismatch = self.get_config_version_mismatch("core")
        payload.web_config_mismatch = self.get_config_version_mismatch("web")
        return payload

    def get_machine_infos(self) -> dict:
        """the function retrieves some information about the host and returns them as a list"""
        infos = {}
        infos["Machine"] = platform.machine()
        infos["Hostname"] = platform.node()
        infos["Platform"] = platform.platform()
        infos["Python Build"] = "".join(platform.python_build())
        infos["Python Compiler"] = platform.python_compiler()
        infos["Python Implementation"] = platform.python_implementation()
        infos["Python Version"] = platform.python_version()
        infos["Release"] = platform.release()
        infos["Libc Version"] = "".join(platform.libc_ver())
        return infos

    def get_config_version_mismatch(self, cfgtype: str) -> bool:
        """
        TODO: Docu
        """
        if cfgtype == "core":
            cfg1 = Config("config/easywall.sample.ini")
            cfg2 = Config("config/easywall.ini")
        elif cfgtype == "web":
            cfg1 = Config("config/web.sample.ini")
            cfg2 = Config("config/web.ini")
        for section in cfg1.get_sections():
            if section not in cfg2.get_sections():
                return True
            for key in cfg1.get_keys(section):
                if key not in cfg2.get_keys(section):
                    return True
        return False

    # -------------------------
    # Update Info Operations

    def get_commit_date(self, datestring: str) -> str:
        """
        the function compares a datetime with the current date
        for comparing the datestring parameter is in UTC timezone
        """
        date1 = datetime.strptime(str(datestring), "%Y-%m-%dT%H:%M:%SZ")
        date1 = date1.replace(
            tzinfo=timezone.utc).astimezone(
                tz=None).replace(
                    tzinfo=None)
        date2 = datetime.now()
        return time_duration_diff(date1, date2)

    def update_last_commit_infos(self) -> None:
        """
        the function retrieves the last commit information after a specific waiting time
        after retrieving the information they are saved into the config file
        """
        currtime = int(time.time())
        lasttime = int(self.cfg.get_value("VERSION", "timestamp"))
        waitseconds = 3600  # 60 minutes × 60 seconds
        if currtime > (lasttime + waitseconds):
            commit = self.get_latest_commit()
            self.cfg.set_value("VERSION", "version", self.get_latest_version())
            self.cfg.set_value("VERSION", "sha", commit["sha"])
            self.cfg.set_value("VERSION", "date", commit["commit"]["author"]["date"])
            self.cfg.set_value("VERSION", "timestamp", str(currtime))

    def get_latest_commit(self) -> Any:
        """
        retrieves the informations of the last commit from github as json
        and converts the information into a python object
        for example the object contains the last commit date and the last commit sha
        This function should not be called very often, because GitHub has a rate limit implemented
        """
        url = "https://api.github.com/repos/jp1337/easywall/commits/master"
        req = urllib.request.Request(
            url,
            data=None,
            headers={
                'User-Agent': 'easywall by github.com/jp1337/easywall'
            }
        )
        response = urllib.request.urlopen(req)
        return json.loads(response.read().decode('utf-8'))

    def get_latest_version(self) -> str:
        """
        the function retrieves the latest version from github and returns the version string
        """
        url = "https://raw.githubusercontent.com/jp1337/easywall/master/.version"
        req = urllib.request.Request(
            url,
            data=None,
            headers={
                'User-Agent': 'easywall by github.com/jp1337/easywall'
            }
        )
        response = urllib.request.urlopen(req)
        data = response.read()
        return str(data.decode('utf-8'))

    # -------------------------
    # Acceptance Operations

    def get_last_accept_time(self) -> str:
        """
        the function retrieves the modify time of the acceptance file
        and compares the time to the current time
        """
        timestamp = str(self.cfg_easywall.get_value("ACCEPTANCE", "timestamp"))
        if timestamp == "":
            return "never"
        timestamp_datetime = datetime.strptime(timestamp, '%Y-%m-%d %H:%M:%S.%f')
        now = datetime.now()
        return time_duration_diff(timestamp_datetime, now)

    def get_acceptance_status(self) -> str:
        """
        get the status of the current acceptance
        """
        filepath = ".acceptance_status"
        if file_exists(filepath):
            return file_get_contents(filepath)
        return ""
