"""the module contains functions for the ports route"""
from flask import render_template, request
from natsort import natsorted
from easywall.rules_handler import RulesHandler
from easywall.web.login import login
from easywall.web.webutils import Webutils


def ports(saved: bool = False) -> str:
    """the function returns the ports page when the user is logged in"""
    utils = Webutils()
    rules = RulesHandler()
    if utils.check_login(request) is True:
        payload = utils.get_default_payload("Open Ports")
        payload.lead = """
            On this page you can open ports for incoming connections.<br />
            You can add tcp and udp ports.<br />
            Please check whether the entries in the list are needed in the future and
            remove old entries if they are no longer needed.<br />
            To list all open ports under Linux use the command <code>netstat -ln</code>
        """
        payload.tcp = natsorted(rules.get_rules_for_web("tcp"))
        payload.udp = natsorted(rules.get_rules_for_web("udp"))
        payload.custom = False
        if rules.diff_new_current("tcp") is True or rules.diff_new_current("udp") is True:
            payload.custom = True
        payload.saved = saved
        return render_template('ports.html', vars=payload)
    return login()


def ports_save() -> str:
    """the function saves the tcp and udp rules into the corresponding rulesfiles"""
    utils = Webutils()
    if utils.check_login(request) is True:
        action = "add"
        ruletype = "tcp"
        port = ""
        ssh = False

        for key, value in request.form.items():
            if key == "remove":
                action = "remove"
                ruletype = value
            elif key == "tcpudp":
                action = "add"
                ruletype = value
            elif key == "port":
                port = str(value)
            elif key == "ssh":
                ssh = True
            else:
                port = str(key)

        if ssh:
            port = "{}#ssh".format(port)

        if action == "add":
            add_port(port, ruletype)
        else:
            remove_port(port, ruletype)

        return ports(True)
    return login()


def add_port(port: str, ruletype: str) -> None:
    """
    The function adds a port to the list of open ports.
    """
    rules = RulesHandler()
    rulelist = rules.get_rules_for_web(ruletype)
    rulelist.append(port)
    rules.save_new_rules(ruletype, rulelist)


def remove_port(port: str, ruletype: str) -> None:
    """
    The function deletes a port from the list of open ports.
    """
    rules = RulesHandler()
    rulelist = rules.get_rules_for_web(ruletype)
    rulelist.remove(port)
    rules.save_new_rules(ruletype, rulelist)
