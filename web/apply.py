"""the module contains functions for the apply rules route"""
from flask import render_template, request

import utility
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
        payload.running = utils.check_acceptance_running()
        payload.accepttime = utils.cfg.get_value("ACCEPTANCE", "time")
        return render_template(
            'apply.html', vars=payload)
    return login("", None)


def apply_save():
    """the function applies the configuration and copies the rules to easywall core"""
    utils = Webutils()
    step = 0
    if utils.check_login() is True:
        for key, value in request.form.items():
            if key == "step_1":
                apply_step_one()
                step = 2
            if key == "step_2":
                apply_step_two()
                step = 3
        return apply(True, step)
    return login("", None)


def apply_step_one():
    """the function copies the rules from temporary web to easywall core"""
    for ruletype in ["blacklist", "whitelist", "tcp", "udp", "custom"]:
        utils = Webutils()
        utils.apply_rule_list(ruletype)


def apply_step_two():
    """the function writes true into the accept file from easywall core"""
    try:
        utils = Webutils()
        filepath = "../" + utils.cfg.get_value("ACCEPTANCE", "filename")
        with open(filepath, mode='wt', encoding='utf-8') as acceptfile:
            acceptfile.write("true")
        for ruletype in ["blacklist", "whitelist", "tcp", "udp", "custom"]:
            utility.delete_file_if_exists(utils.get_rule_file_path(ruletype, True))
    except Exception as exc:
        print("{}".format(exc))
