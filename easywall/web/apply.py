"""
the module contains functions for the apply rules route
"""
from datetime import datetime

from flask import render_template, request
from easywall.utility import create_file_if_not_exists, write_into_file
from easywall.web.login import login
from easywall.web.webutils import Webutils


def apply(saved: bool = False, step: int = 1) -> str:
    """
    the function returns the apply page when the user is logged in
    """
    utils = Webutils()
    if utils.check_login(request) is True:
        payload = utils.get_default_payload("Apply")
        payload.lead = """
            The defined firewall rules were not automatically activated for security reasons.<br>
            On this page, you can safely apply the defined rules.<br>
            The activation takes place in two steps and an exclusion from the server
            should be prevented.
            """
        payload.saved = saved
        payload.step = step
        payload.lastapplied = utils.get_last_accept_time()
        payload.running = step > 1
        payload.accepttime = str(utils.cfg_easywall.get_value("ACCEPTANCE", "duration"))
        return render_template('apply.html', vars=payload)
    return login()


def apply_save() -> str:
    """
    the function applies the configuration and copies the rules to easywall core
    """
    utils = Webutils()
    step = 0
    if utils.check_login(request) is True:
        for key, value in request.form.items():
            if key == "step_1":
                apply_step_one()
                step = 2
            if key == "step_2":
                apply_step_two()
                step = 3
        return apply(True, step)
    return login()


def apply_step_one() -> None:
    """
    the function triggeres the easywall core to apply the new firewall rules
    """
    create_file_if_not_exists(".apply")


def apply_step_two() -> None:
    """the function writes true into the accept file from easywall core"""
    write_into_file(".acceptance", "true")
    utils = Webutils()
    utils.cfg_easywall.set_value("ACCEPTANCE", "timestamp",
                                 datetime.now().strftime('%Y-%m-%d %H:%M:%S.%f'))
