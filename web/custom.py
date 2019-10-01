from webutils import webutils
from login import login
from flask import render_template, request


def custom(saved=False):
    """the function returns the custom rules page when the user is logged in"""
    utils = webutils()
    if utils.check_login() is True:
        payload = utils.get_default_payload("Custom")
        payload.custom = utils.get_rule_list("custom")
        payload.saved = saved
        return render_template(
            'custom.html', vars=payload)
    return login("", None)


def custom_save():
    """the function saves the custom rules into the corresponding rulesfile"""
    utils = webutils()
    if utils.check_login() is True:
        for key, value in request.form.items():
            rulelist = value.split("\n")
            utils.save_rule_list("custom", rulelist)
        return custom(True)
    return login("", None)
