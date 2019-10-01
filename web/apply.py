from webutils import webutils
from login import login
from flask import render_template


def apply(saved=False, step=1):
    """the function returns the apply page when the user is logged in"""
    utils = webutils()
    if utils.check_login() is True:
        payload = utils.get_default_payload("Apply")
        payload.saved = saved
        payload.step = step
        payload.lastapplied = utils.get_last_accept_time()
        payload.accepttime = utils.cfg.get_value("ACCEPTANCE", "time")
        return render_template(
            'apply.html', vars=payload)
    return login("", None)
