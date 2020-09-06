"""The module contains a empty class which is used as object."""
from easywall.config import Config


class DefaultPayload(object):
    """The class is a empty skeleton for generating objects."""

    def __init__(self) -> None:
        """TODO: Doku."""
        self.config: Config
        self.config_web: Config
        self.config_log: Config
        self.saved: bool
        self.addresses: list
        self.tcp: list
        self.udp: list
        self.forwardings: list
        self.rules: list
        self.messagetype: str
        self.message: str
        self.year: int
        self.title: str
        self.custom: bool
        self.lead: str
        self.customcss: str
        self.machine: dict
        self.latest_version: str
        self.current_version: str
        self.commit_sha: str
        self.commit_date: str
        self.config_mismatch: bool
        self.web_config_mismatch: bool
        self.error: str
        self.error_code: int
        self.error_desc: str
        self.step: int
        self.lastapplied: str
        self.running: bool
        self.accepttime: str
        self.active_tab: str
