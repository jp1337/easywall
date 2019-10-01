"""the module contains functions for the blacklist route"""
from flask import render_template, request

from login import login
from webutils import Webutils


def blacklist(saved=False):
    """the function returns the blacklist page when the user is logged in"""
    utils = Webutils()
    if utils.check_login() is True:
        payload = utils.get_default_payload("Blacklist")
        payload.addresses = utils.get_rule_list("blacklist")
        payload.saved = saved
        return render_template(
            'blacklist.html', vars=payload)
    return login("", None)


def blacklist_save():
    """the function saves the blacklist rules into the corresponding rulesfile"""
    utils = Webutils()
    if utils.check_login() is True:
        ipaddress = ""
        rulelist = utils.get_rule_list("blacklist")

        for key, value in request.form.items():
            if key == "ipadr":
                # then a new ip address is blacklisted
                ipaddress = value
                rulelist.append(ipaddress)
                utils.save_rule_list("blacklist", rulelist)
            else:
                # then a old ip address is removed
                ipaddress = key
                rulelist.remove(ipaddress)
                utils.save_rule_list("blacklist", rulelist)
        return blacklist(True)
    return login("", None)
