"""the module contains functions for custom error routes"""
from typing import Tuple, Union

from flask import render_template, request

from easywall_web.login import login
from easywall_web.webutils import Webutils


def page_not_found(error: int) -> Union[str, Tuple[str, int]]:
    """the function returns the 404 error page when the user is logged in"""
    utils = Webutils()
    if utils.check_login(request) is True:
        return render_template(
            '404.html', vars=utils.get_default_payload("404 Error", "error")), 404
    return login()
