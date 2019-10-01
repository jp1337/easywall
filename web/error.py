from webutils import webutils
from login import login
from flask import render_template


def page_not_found(error):
    """the function returns the 404 error page when the user is logged in"""
    utils = webutils()
    if utils.check_login() is True:
        return render_template(
            '404.html', vars=utils.get_default_payload("404 Error", "error")), 404
    return login("", None)
