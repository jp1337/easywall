"""the app module contains all information of the Flask app"""
import os
from logging import info
from typing import Tuple, Union

from flask import Flask
from werkzeug.wrappers import Response

from easywall.config import Config
from easywall.log import Log
from easywall.rules_handler import RulesHandler
from easywall.utility import folder_exists
from easywall_web.apply import apply, apply_save
from easywall_web.blacklist import blacklist, blacklist_save
from easywall_web.custom import custom, custom_save
from easywall_web.error import page_not_found
from easywall_web.forwarding import forwarding, forwarding_save
from easywall_web.index import index
from easywall_web.login import login_post, logout
from easywall_web.options import options, options_save
from easywall_web.ports import ports, ports_save
from easywall_web.whitelist import whitelist, whitelist_save

APP = Flask(__name__)
CONFIG_PATH = "config/web.ini"


@APP.after_request
def apply_headers(response: Response) -> Response:
    """
    TODO: Docu
    """
    response.headers["X-Frame-Options"] = "DENY"
    return response


@APP.route('/')
def index_route() -> str:
    """The function calls the corresponding function from the appropriate module"""
    return index()


@APP.route('/options')
def options_route() -> str:
    """The function calls the corresponding function from the appropriate module"""
    return options()


@APP.route('/options-save', methods=['POST'])
def options_save_route() -> str:
    """The function calls the corresponding function from the appropriate module"""
    return options_save()


@APP.route('/blacklist')
def blacklist_route() -> str:
    """The function calls the corresponding function from the appropriate module"""
    return blacklist()


@APP.route('/blacklist-save', methods=['POST'])
def blacklist_save_route() -> str:
    """The function calls the corresponding function from the appropriate module"""
    return blacklist_save()


@APP.route('/whitelist')
def whitelist_route() -> str:
    """The function calls the corresponding function from the appropriate module"""
    return whitelist()


@APP.route('/whitelist-save', methods=['POST'])
def whitelist_save_route() -> str:
    """The function calls the corresponding function from the appropriate module"""
    return whitelist_save()


@APP.route('/forwarding')
def forwarding_route() -> str:
    """The function calls the corresponding function from the appropriate module"""
    return forwarding()


@APP.route('/forwarding-save', methods=['POST'])
def forwarding_save_route() -> str:
    """The function calls the corresponding function from the appropriate module"""
    return forwarding_save()


@APP.route('/ports')
def ports_route() -> str:
    """The function calls the corresponding function from the appropriate module"""
    return ports()


@APP.route('/ports-save', methods=['POST'])
def ports_save_route() -> str:
    """The function calls the corresponding function from the appropriate module"""
    return ports_save()


@APP.route('/custom')
def custom_route() -> str:
    """The function calls the corresponding function from the appropriate module"""
    return custom()


@APP.route('/custom-save', methods=['POST'])
def custom_save_route() -> str:
    """The function calls the corresponding function from the appropriate module"""
    return custom_save()


@APP.route('/apply')
def apply_route() -> str:
    """The function calls the corresponding function from the appropriate module"""
    return apply()


@APP.route('/apply-save', methods=['POST'])
def apply_save_route() -> str:
    """The function calls the corresponding function from the appropriate module"""
    return apply_save()


@APP.route('/login', methods=['POST'])
def login_post_route() -> Union[Response, str]:
    """The function calls the corresponding function from the appropriate module"""
    return login_post()


@APP.route("/logout")
def logout_route() -> str:
    """The function calls the corresponding function from the appropriate module"""
    return logout()


@APP.errorhandler(404)
def page_not_found_route(error: int) -> Union[str, Tuple[str, int]]:
    """The function calls the corresponding function from the appropriate module"""
    return page_not_found(error)


class DefaultConfig(object):
    """
    TODO: Docu
    """
    DEBUG = False
    TESTING = False
    SECRET_KEY = os.urandom(265)
    ENV = "production"
    SESSION_COOKIE_NAME = "easywall"
    SESSION_COOKIE_SECURE = True
    SESSION_COOKIE_HTTPONLY = True
    SESSION_COOKIE_SAMESITE = "Lax"
    PERMANENT_SESSION_LIFETIME = 600
    PREFERRED_URL_SCHEME = "https"
    MAX_CONTENT_LENGTH = 10240


class ProductionConfig(DefaultConfig):
    """
    TODO: Docu
    """


class DevelopmentConfig(DefaultConfig):
    """
    TODO: Docu
    """
    DEBUG = True
    ENV = "development"


class Main(object):
    """
    TODO: Doku
    """

    def __init__(self, debug: bool = False) -> None:
        self.cfg = Config(CONFIG_PATH)

        loglevel = self.cfg.get_value("LOG", "level")
        to_stdout = self.cfg.get_value("LOG", "to_stdout")
        to_files = self.cfg.get_value("LOG", "to_files")
        logpath = self.cfg.get_value("LOG", "filepath")
        logfile = self.cfg.get_value("LOG", "filename")
        self.log = Log(str(loglevel), bool(to_stdout), bool(to_files), str(logpath), str(logfile))

        info("starting easywall-web")

        self.is_first_run = not folder_exists("rules")
        self.rules_handler = RulesHandler()
        if self.is_first_run:
            self.rules_handler.ensure_files_exist()

        if debug is True:
            port = self.cfg.get_value("WEB", "bindport")
            host = self.cfg.get_value("WEB", "bindip")
            APP.config.from_object('easywall_web.__main__.DevelopmentConfig')
            APP.run(str(host), str(port))
        else:
            APP.config.from_object('easywall_web.__main__.ProductionConfig')


if __name__ == '__main__':
    Main(debug=True)
else:
    Main()
