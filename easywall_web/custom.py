"""the module contains functions for the custom rules route"""
from flask import render_template, request
from easywall_web.login import login
from easywall_web.webutils import Webutils
from easywall.rules_handler import RulesHandler


def custom(saved=False):
    """the function returns the custom rules page when the user is logged in"""
    utils = Webutils()
    rules = RulesHandler()
    if utils.check_login() is True:
        payload = utils.get_default_payload("Custom")
        payload.rules = rules.get_rules_for_web("custom")
        payload.custom = rules.diff_new_current("custom")
        payload.saved = saved
        return render_template('custom.html', vars=payload)
    return login("", None)


def custom_save():
    """the function saves the custom rules into the corresponding rulesfile"""
    utils = Webutils()
    rules = RulesHandler()
    if utils.check_login() is True:
        for key, value in request.form.items():
            key = str(key) + ""  # just for ignoring the warning
            rulelist = value.split("\n")
            rules.save_new_rules("custom", rulelist)
        return custom(True)
    return login("", None)
