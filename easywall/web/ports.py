"""Contains functions for the ports route."""
from operator import itemgetter

from easywall.rules_handler import RulesHandler
from easywall.web.login import login
from easywall.web.webutils import Webutils
from flask import render_template, request
from natsort import natsorted


def ports(saved: bool = False) -> str:
    """Return the ports page when the user is logged in."""
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
        payload.tcp = natsorted(rules.get_rules_for_web("tcp"), key=itemgetter(*['port']))
        payload.udp = natsorted(rules.get_rules_for_web("udp"), key=itemgetter(*['port']))
        payload.custom = False
        if rules.diff_new_current("tcp") is True or rules.diff_new_current("udp") is True:
            payload.custom = True
        payload.saved = saved
        return render_template('ports.html', vars=payload)
    return login()


def ports_save() -> str:
    """Save the tcp and udp rules into the corresponding rulesfiles."""
    utils = Webutils()
    if utils.check_login(request) is True:
        action = "add"

        entry: dict = {}
        entry["ruletype"] = "tcp"
        entry["port"] = ""
        entry["description"] = ""
        entry["ssh"] = False

        for key, value in request.form.items():
            if key == "remove":
                action = "remove"
                entry["ruletype"] = value
            elif key == "tcpudp":
                action = "add"
                entry["ruletype"] = value
            elif key == "port":
                entry["port"] = value
            elif key == "description":
                entry["description"] = value
            elif key == "ssh":
                entry["ssh"] = True
            else:
                entry["port"] = key

        result = True
        if action == "add":
            result = add_port(entry)
        else:
            result = remove_port(entry)

        return ports(result)
    return login()


def add_port(entry: dict) -> bool:
    """Add a port to the list of open ports."""
    rules = RulesHandler()
    ruletype = entry["ruletype"]
    rulelist = rules.get_rules_for_web(ruletype)
    entry.pop("ruletype", None)  # we dont't want the ruletype to be saved
    duplicate = False
    for i in range(len(rulelist)):
        if rulelist[i]['port'] == entry["port"]:
            duplicate = True
            break
    if duplicate is False:
        rulelist.append(entry)
        rules.save_new_rules(ruletype, rulelist)
        return True
    return False


def remove_port(entry: dict) -> bool:
    """Delete a port from the list of open ports."""
    rules = RulesHandler()
    rulelist = rules.get_rules_for_web(entry["ruletype"])
    for i in range(len(rulelist)):
        if rulelist[i]['port'] == entry["port"]:
            del rulelist[i]
            break
    rules.save_new_rules(entry["ruletype"], rulelist)
    return True
