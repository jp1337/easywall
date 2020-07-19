"""the module contains functions for the options route"""
from hashlib import sha512
from platform import node

from flask import render_template, request

from easywall_web.login import login
from easywall_web.webutils import Webutils


def options(saved: bool = False, error: str = "") -> str:
    """the function returns the options page when the user is logged in"""
    utils = Webutils()
    if utils.check_login(request) is True:
        payload = utils.get_default_payload("Options")
        payload.lead = """
            On this page you can configure easywall.<br />
            All entries from the configuration files for the core
            and the web interface are available.<br />
            Some entries require a restart of the respective program part.
        """
        payload.config = utils.cfg_easywall
        payload.config_web = utils.cfg
        payload.saved = saved
        payload.error = error
        return render_template('options.html', vars=payload)
    return login()


def options_save() -> str:
    """
    the function saves the options from a section using the config class
    for example the Enabled flag in the IPv6 section is saved to the config file
    """
    utils = Webutils()
    if utils.check_login(request) is True:
        section = request.form['section']
        cfgtype = request.form['cfgtype']
        password1 = ""
        password2 = ""

        for key, value in request.form.items():
            if key != "section" and key != "cfgtype":
                if key.startswith("checkbox"):
                    key = key.replace("checkbox_", "")
                    value = correct_value_checkbox(key)
                if key.startswith("password1"):
                    password1 = value
                if key.startswith("password2"):
                    password2 = value
                if value != "on" and not key.startswith("password"):
                    if cfgtype == "easywall":
                        utils.cfg_easywall.set_value(section, key, value)
                    elif cfgtype == "web":
                        utils.cfg.set_value(section, key, value)
                    else:
                        return options(
                            saved=False,
                            error="The configuration could not be saved due to invalid parameters.")

        if password1:
            if password1 == password2:
                hostname = node().encode("utf-8")
                salt = sha512(hostname).hexdigest()
                pw_hash = sha512(str(salt + password1).encode("utf-8")).hexdigest()
                utils.cfg_easywall.set_value("WEB", "password", pw_hash)
            else:
                return options(saved=False, error="The entered passwords are not identical.")

        return options(saved=True)
    return login()


def correct_value_checkbox(key: str) -> str:
    """the function corrects the value of a checkbox"""
    key = key.replace("checkbox_", "")
    if key in request.form:
        return "yes"
    return "no"
