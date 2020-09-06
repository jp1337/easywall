"""TODO: Doku."""
from hashlib import sha512
from platform import node
from typing import Union

from flask import redirect, render_template, request
from werkzeug.wrappers import Response

from easywall.web.login import login
from easywall.web.webutils import Webutils


def firstrun(message: Union[None, str] = None,
             messagetype: Union[None, str] = None) -> Union[Response, str]:
    """TODO: Doku."""
    utils = Webutils()
    if utils.check_first_run() is False:
        return redirect("/")
    payload = utils.get_default_payload("Welcome to easywall!", "signin")
    if messagetype is not None:
        payload.messagetype = messagetype
    if message is not None:
        payload.message = message
    return render_template('firstrun.html', vars=payload)


def firstrun_save() -> Union[Response, str]:
    """TODO: Doku."""
    utils = Webutils()
    if utils.check_first_run() is False:
        return redirect("/")
    username = request.form['username']
    password = request.form['password']
    passwordconf = request.form['password-confirm']
    if password != passwordconf:
        return firstrun("The passwords are not the same. Please try again.", "danger")
    try:
        utils.cfg.set_value("WEB", "username", username)
        hostname = node().encode("utf-8")
        salt = sha512(hostname).hexdigest()
        pw_hash = sha512(str(salt + password).encode("utf-8")).hexdigest()
        utils.cfg.set_value("WEB", "password", pw_hash)
        return login("Username and password have been successfully saved. "
                     "Please log in now with your access data.", "success")
    except Exception:
        return firstrun("An error occurred while saving username or password. "
                        "Please check the logfile for errors.", "danger")
