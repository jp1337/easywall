from webutils import webutils
from login import login
from flask import render_template, request


def whitelist(saved=False):
    """the function returns the whitelist page when the user is logged in"""
    utils = webutils()
    if utils.check_login() is True:
        payload = utils.get_default_payload("Whitelist")
        payload.addresses = utils.get_rule_list("whitelist")
        payload.saved = saved
        return render_template(
            'whitelist.html', vars=payload)
    return login("", None)


def whitelist_save():
    """the function saves the whitelist rules into the corresponding rulesfile"""
    utils = webutils()
    if utils.check_login() is True:
        ipaddress = ""
        rulelist = utils.get_rule_list("whitelist")

        for key, value in request.form.items():
            if key == "ipadr":
                # then a new ip address is whitelisted
                ipaddress = value
                rulelist.append(ipaddress)
                utils.save_rule_list("whitelist", rulelist)
            else:
                # then a old ip address is removed
                ipaddress = key
                rulelist.remove(ipaddress)
                utils.save_rule_list("whitelist", rulelist)
        return whitelist(True)
    return login("", None)
