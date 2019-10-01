"""the module contains functions for the options route"""
from flask import render_template, request

from login import login
from webutils import Webutils


def options(saved=False):
    """the function returns the options page when the user is logged in"""
    utils = Webutils()
    if utils.check_login() is True:
        payload = utils.get_default_payload("Options")
        payload.config = utils.cfg
        payload.saved = saved
        return render_template(
            'options.html', vars=payload)
    return login("", None)


def options_save():
    """
    the function saves the options from a section using the config class
    for example the Enabled flag in the IPv6 section is saved to the config file
    """
    utils = Webutils()
    if utils.check_login() is True:
        section = request.form['section']
        for key, value in request.form.items():
            if key != "section":
                # checkbox workaround.
                if key.startswith("checkbox"):
                    key = key.replace("checkbox_", "")
                    if key in request.form:
                        value = "yes"
                    else:
                        value = "no"
                if value != "on":
                    utils.cfg.set_value(section, key, value)
        return options(True)
    return login("", None)
