"""the module contains functions for the ports route"""
from flask import render_template, request

from login import login
from webutils import Webutils


def ports(saved=False):
    """the function returns the ports page when the user is logged in"""
    utils = Webutils()
    if utils.check_login() is True:
        payload = utils.get_default_payload("Ports")
        payload.tcp = utils.get_rule_list("tcp")
        payload.udp = utils.get_rule_list("udp")
        payload.saved = saved
        return render_template(
            'ports.html', vars=payload)
    return login("", None)


def ports_save():
    """the function saves the tcp and udp rules into the corresponding rulesfiles"""
    utils = Webutils()
    if utils.check_login() is True:
        tcp = utils.get_rule_list("tcp")
        udp = utils.get_rule_list("udp")
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
            if ruletype == "tcp":
                tcp.append(port)
                utils.save_rule_list("tcp", tcp)
            else:
                udp.append(port)
                utils.save_rule_list("udp", udp)
        else:
            if ruletype == "tcp":
                tcp.remove(port)
                utils.save_rule_list("tcp", tcp)
            else:
                udp.remove(port)
                utils.save_rule_list("udp", udp)

        return ports(True)
    return login("", None)
