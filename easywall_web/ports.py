"""the module contains functions for the ports route"""
from flask import render_template, request
from easywall_web.login import login
from easywall_web.webutils import Webutils


def ports(saved=False):
    """the function returns the ports page when the user is logged in"""
    utils = Webutils()
    if utils.check_login() is True:
        payload = utils.get_default_payload("Ports")
        payload.tcp = utils.get_rule_list("tcp")
        payload.udp = utils.get_rule_list("udp")
        payload.custom = False
        if utils.get_rule_status("tcp") == "custom" or utils.get_rule_status("udp") == "custom":
            payload.custom = True
        payload.saved = saved
        return render_template(
            'ports.html', vars=payload)
    return login("", None)


def ports_save():
    """the function saves the tcp and udp rules into the corresponding rulesfiles"""
    utils = Webutils()
    if utils.check_login() is True:
        action = "add"
        ruletype = "tcp"
        port = ""

        for key, value in request.form.items():
            if key == "remove":
                action = "remove"
                ruletype = value
            elif key == "tcpudp":
                action = "add"
                ruletype = value
            elif key == "port":
                port = str(value)
            else:
                port = str(key)

        if action == "add":
            add_port(port, ruletype)
        else:
            remove_port(port, ruletype)

        return ports(True)
    return login("", None)


def add_port(port, ruletype):
    """the function adds a port to the opened port rules file"""
    utils = Webutils()
    rulelist = utils.get_rule_list(ruletype)
    rulelist.append(port)
    utils.save_rule_list(ruletype, rulelist)


def remove_port(port, ruletype):
    """the function removes a port from the opened port rules file"""
    utils = Webutils()
    rulelist = utils.get_rule_list(ruletype)
    rulelist.remove(port)
    utils.save_rule_list(ruletype, rulelist)
