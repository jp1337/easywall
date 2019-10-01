"""the module contains functions for custom error routes"""
from flask import render_template

from login import login
from webutils import Webutils


def page_not_found(error):
    """the function returns the 404 error page when the user is logged in"""
    error += ""  # just for removing the warning
    utils = Webutils()
    if utils.check_login() is True:
        return render_template(
            '404.html', vars=utils.get_default_payload("404 Error", "error")), 404
    return login("", None)
