"""the module contains functions for the custom rules route"""
from flask import render_template, request
from easywall.web.login import login
from easywall.web.webutils import Webutils
from easywall.rules_handler import RulesHandler


def custom(saved: bool = False) -> str:
    """the function returns the custom rules page when the user is logged in"""
    utils = Webutils()
    rules = RulesHandler()
    if utils.check_login(request) is True:
        payload = utils.get_default_payload("Custom")
        payload.lead = """
            On this page you can add your own firewall rules.<br />
            Please check the rules for accuracy, as these are not tested by easywall.<br />
            <br />
            To add your own rule, simply copy the rule into the text box. One rule per line.<br />
            It is important to omit the iptables command.<br />
            Example: <code>-P FORWARD DROP</code>
        """
        payload.rules = rules.get_rules_for_web("custom")
        payload.custom = rules.diff_new_current("custom")
        payload.saved = saved
        return render_template('custom.html', vars=payload)
    return login()


def custom_save() -> str:
    """the function saves the custom rules into the corresponding rulesfile"""
    utils = Webutils()
    rules = RulesHandler()
    if utils.check_login(request) is True:
        for key, value in request.form.items():
            key = str(key) + ""  # just for ignoring the warning
            rulelist = value.split("\n")
            rules.save_new_rules("custom", rulelist)
        return custom(True)
    return login()
