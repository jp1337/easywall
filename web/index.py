"""the module contains functions for the index route"""
from flask import render_template

from login import login
from webutils import Webutils


def index():
    """the function returns the index page when the user is logged in"""
    utils = Webutils()
    if utils.check_login() is True:
        return render_template('index.html', vars=utils.get_default_payload("Home"))
    return login("", None)
