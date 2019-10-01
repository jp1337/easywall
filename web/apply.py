"""the module contains functions for the apply rules route"""
from flask import render_template

from login import login
from webutils import Webutils


def apply(saved=False, step=1):
    """the function returns the apply page when the user is logged in"""
    utils = Webutils()
    if utils.check_login() is True:
        payload = utils.get_default_payload("Apply")
        payload.saved = saved
        payload.step = step
        payload.lastapplied = utils.get_last_accept_time()
        payload.accepttime = utils.cfg.get_value("ACCEPTANCE", "time")
        return render_template(
            'apply.html', vars=payload)
    return login("", None)
