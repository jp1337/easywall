"""the module contains functions for the blacklist route"""
from flask import render_template, request
from easywall.web.login import login
from easywall.web.webutils import Webutils
from easywall.rules_handler import RulesHandler


def blacklist(saved: bool = False) -> str:
    """the function returns the blacklist page when the user is logged in"""
    utils = Webutils()
    rules = RulesHandler()
    if utils.check_login(request) is True:
        payload = utils.get_default_payload("Blacklist")
        payload.lead = """
            On this page you can list IP addresses that are not allowed to connect to this machine.
            <br />
            Please check the IP addresses carefully, as they are not checked by easywall.<br />
            You can add IPv4 and IPv6 addresses to the list.
        """
        payload.addresses = rules.get_rules_for_web("blacklist")
        payload.custom = rules.diff_new_current("blacklist")
        payload.saved = saved
        return render_template('blacklist.html', vars=payload)
    return login()


def blacklist_save() -> str:
    """
    the function saves the blacklist rules into the corresponding rulesfile
    """
    utils = Webutils()
    rules = RulesHandler()
    if utils.check_login(request) is True:
        ipaddress = ""
        rulelist = rules.get_rules_for_web("blacklist")

        for key, value in request.form.items():
            if key == "ipadr":
                # then a new ip address is blacklisted
                ipaddress = value
                rulelist.append(ipaddress)
                rules.save_new_rules("blacklist", rulelist)
            else:
                # then a old ip address is removed
                ipaddress = key
                rulelist.remove(ipaddress)
                rules.save_new_rules("blacklist", rulelist)
        return blacklist(True)
    return login()
