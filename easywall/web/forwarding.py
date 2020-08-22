"""TODO: Doku."""
from flask import render_template, request
from easywall.web.login import login
from easywall.web.webutils import Webutils
from easywall.rules_handler import RulesHandler


def forwarding(saved: bool = False) -> str:
    """TODO: Doku."""
    utils = Webutils()
    rules = RulesHandler()
    if utils.check_login(request):
        payload = utils.get_default_payload("Port Forwarding")
        payload.lead = """
            This page allows you to forward ports from the local system to ports on the
            Internet.<br />
            This is especially useful if the port of an application cannot be changed.<br />
            Enter the port type, source and destination.<br />
            You do not have to release the public port separately, easywall will do that for you.
        """
        payload.forwardings = rules.get_rules_for_web("forwarding")
        payload.custom = False
        if rules.diff_new_current("forwarding"):
            payload.custom = True
        payload.saved = saved
        return render_template('forwarding.html', vars=payload)
    return login()


def forwarding_save() -> str:
    """TODO: Doku."""
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
                dest_port = str(value)
            else:
                if ":" in key:
                    ruletype = key.split(":")[0]
                    source_port = key.split(":")[1]
                    dest_port = key.split(":")[2]

        if action == "add":
            add_forwarding(source_port, dest_port, ruletype)
        else:
            remove_forwarding(source_port, dest_port, ruletype)

        return forwarding(True)
    return login()


def add_forwarding(source_port: str, dest_port: str, ruletype: str) -> None:
    """TODO: Doku."""
    rules = RulesHandler()
    rulelist = rules.get_rules_for_web("forwarding")
    rulelist.append("{}:{}:{}".format(ruletype, source_port, dest_port))
    rules.save_new_rules("forwarding", rulelist)


def remove_forwarding(source_port: str, dest_port: str, ruletype: str) -> None:
    """TODO: Doku."""
    rules = RulesHandler()
    rulelist = rules.get_rules_for_web("forwarding")
    rulelist.remove("{}:{}:{}".format(ruletype, source_port, dest_port))
    rules.save_new_rules("forwarding", rulelist)
