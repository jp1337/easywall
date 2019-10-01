from flask import render_template
from webutils import webutils
from login import login


def index():
    """the function returns the index page when the user is logged in"""
    utils = webutils()
    if utils.check_login() is True:
        return render_template('index.html', vars=utils.get_default_payload("Home"))
    return login("", None)
