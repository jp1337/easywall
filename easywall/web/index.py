"""The module contains functions for the index route."""
from flask import render_template, request
from easywall.web.login import login
from easywall.web.webutils import Webutils


def index() -> str:
    """Return the index page when the user is logged in."""
    utils = Webutils()
    if utils.check_login(request) is True:
        return render_template('index.html', vars=utils.get_default_payload("Home"))
    return login()
