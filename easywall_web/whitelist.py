"""the module contains functions for the whitelist route"""
from flask import render_template, request
from easywall_web.login import login
from easywall_web.webutils import Webutils
from easywall.rules_handler import RulesHandler


def whitelist(saved=False):
    """the function returns the whitelist page when the user is logged in"""
    utils = Webutils()
    rules = RulesHandler()
    if utils.check_login(request) is True:
        payload = utils.get_default_payload("Whitelist")
        payload.lead = """
            On this page you can add IP-addresses that are allowed to connect to this server.<br />
            Use this carefully because there is no validation of IP-adresses..<br />
            You can add IPv4 and IPv6 adresses here.
        """
        payload.addresses = rules.get_rules_for_web("whitelist")
        payload.custom = rules.diff_new_current("whitelist")
        payload.saved = saved
        return render_template('whitelist.html', vars=payload)
    return login("", None)


def whitelist_save():
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
    return login("", None)
