"""the module contains functions for the whitelist route"""
from flask import render_template, request
from easywall.web.login import login
from easywall.web.webutils import Webutils
from easywall.rules_handler import RulesHandler


def whitelist(saved: bool = False) -> str:
    """the function returns the whitelist page when the user is logged in"""
    utils = Webutils()
    rules = RulesHandler()
    if utils.check_login(request) is True:
        payload = utils.get_default_payload("Whitelist")
        payload.lead = """
            On this page you can list IP addresses that are always allowed to connect to all
            ports of the system.<br />
            Please check the IP addresses carefully, as they are not checked by easywall.<br />
            You can add IPv4 and IPv6 addresses to the list.
        """
        payload.addresses = rules.get_rules_for_web("whitelist")
        payload.custom = rules.diff_new_current("whitelist")
        payload.saved = saved
        return render_template('whitelist.html', vars=payload)
    return login()


def whitelist_save() -> str:
    """
    the function saves the whitelist rules into the corresponding rulesfile
    """
    utils = Webutils()
    rules = RulesHandler()
    if utils.check_login(request) is True:
        ipaddress = ""
        rulelist = rules.get_rules_for_web("whitelist")

        for key, value in request.form.items():
            if key == "ipadr":
                # then a new ip address is whitelisted
                ipaddress = value
                rulelist.append(ipaddress)
                rules.save_new_rules("whitelist", rulelist)
            else:
                # then a old ip address is removed
                ipaddress = key
                rulelist.remove(ipaddress)
                rules.save_new_rules("whitelist", rulelist)
        return whitelist(True)
    return login()
