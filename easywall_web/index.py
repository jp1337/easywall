"""
the module contains functions for the index route
"""
from flask import render_template
from easywall_web.login import login
from easywall_web.webutils import Webutils


def index():
    """
    the function returns the index page when the user is logged in
    """
    utils = Webutils()
    if utils.check_login() is True:
        return render_template('index.html', vars=utils.get_default_payload("Home"))
    return login("", None)
