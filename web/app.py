"""the app module contains all information of the Flask app"""
import os

from flask import Flask

from apply import apply
from blacklist import blacklist, blacklist_save
from config import Config
from custom import custom, custom_save
from error import page_not_found
from index import index
from login import login_post, logout
from options import options, options_save
from ports import ports, ports_save
from whitelist import whitelist, whitelist_save

APP = Flask(__name__)
CFG = Config("../config/config.ini")


@APP.route('/')
def index_route():
    return index()


@APP.route('/options')
def options_route():
    return options()


@APP.route('/blacklist')
def blacklist_route():
    return blacklist()


@APP.route('/whitelist')
def whitelist_route():
    return whitelist()


@APP.route('/ports')
def ports_route():
    return ports()


@APP.route('/custom')
def custom_route():
    return custom()


@APP.route('/apply')
def apply_route():
    return apply()


@APP.route('/options-save', methods=['POST'])
def options_save_route():
    return options_save()


@APP.route('/blacklist-save', methods=['POST'])
def blacklist_save_route():
    return blacklist_save()


@APP.route('/whitelist-save', methods=['POST'])
def whitelist_save_route():
    return whitelist_save()


@APP.route('/ports-save', methods=['POST'])
def ports_save_route():
    return ports_save()


@APP.route('/custom-save', methods=['POST'])
def custom_save_route():
    return custom_save()


@APP.route('/login', methods=['POST'])
def login_post_route():
    return login_post()


@APP.route("/logout")
def logout_route():
    return logout()


@APP.errorhandler(404)
def page_not_found_route(error):
    return page_not_found(error)


# only debugging
if __name__ == '__main__':
    APP.secret_key = os.urandom(12)
    PORT = int(CFG.get_value("WEB", "bindport"))
    HOST = CFG.get_value("WEB", "bindip")
    DEBUG = True
    APP.run(HOST, PORT, DEBUG)

# production mode
if __name__ == 'uwsgi_file_app':
    APP.secret_key = os.urandom(12)
