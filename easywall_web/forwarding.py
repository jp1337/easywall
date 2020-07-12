"""
TODO: Docu
"""
from flask import render_template, request
from easywall_web.login import login
from easywall_web.webutils import Webutils
from easywall.rules_handler import RulesHandler


def forwarding(saved=False):
    """
    TODO: Docu
    """
    utils = Webutils()
    rules = RulesHandler()
    if utils.check_login(request):
        payload = utils.get_default_payload("Forwarding")
        payload.forwarding = rules.get_rules_for_web("forwarding")
        payload.custom = False
        if rules.diff_new_current("forwarding"):
            payload.custom = True
        payload.saved = saved
        return render_template('forwarding.html', vars=payload)
    return login("", None)


def forwarding_save():
    """
    TODO: Docu
    """
    utils = Webutils()
    if utils.check_login(request) is True:
        action = "add"
        ruletype = "tcp"
        source_port = ""
        dest_port = ""

        for key, value in request.form.items():
            if key == "remove":
                action = "remove"
                ruletype = value
            elif key == "tcpudp":
                action = "add"
                ruletype = value
            elif key == "source-port":
                source_port = str(value)
            elif key == "destination-port":
                source_port = str(value)

        if action == "add":
            add_forwarding(source_port, dest_port, ruletype)
        else:
            remove_forwarding(source_port, dest_port, ruletype)

        return forwarding(True)
    return login("", None)


def add_forwarding(source_port: str, dest_port: str, ruletype: str):
    """
    TODO: Docu
    """
    rules = RulesHandler()
    rulelist = rules.get_rules_for_web("forwarding")
    rulelist.append("{}:{}:{}".format(ruletype, source_port, dest_port))
    rules.save_new_rules(ruletype, rulelist)


def remove_forwarding(source_port: str, dest_port: str, ruletype: str):
    """
    TODO: Docu
    """
    rules = RulesHandler()
    rulelist = rules.get_rules_for_web("forwarding")
    rulelist.remove("{}:{}:{}".format(ruletype, source_port, dest_port))
    rules.save_new_rules(ruletype, rulelist)
