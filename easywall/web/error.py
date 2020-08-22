"""the module contains functions for custom error routes"""
from typing import Tuple, Union

from flask import render_template, request

from easywall.web.login import login
from easywall.web.webutils import Webutils


def page_not_found(error: str) -> Union[str, Tuple[str, int]]:
    """the function returns the 404 error page when the user is logged in"""
    utils = Webutils()
    if utils.check_login(request) is True:
        payload = utils.get_default_payload("404 Error", "error")
        payload.error_code = 404
        payload.error_desc = error
        return render_template('error.html', vars=payload), 404
    return login()


def forbidden(error: str) -> Union[str, Tuple[str, int]]:
    """the function returns the 403 error page when the user is logged in"""
    utils = Webutils()
    payload = utils.get_default_payload("403 Error", "error")
    payload.error_code = 403
    payload.error_desc = error
    return render_template('error.html', vars=payload), 403
