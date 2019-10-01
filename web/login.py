from webutils import webutils
from flask import render_template, redirect, session, request
import hashlib
import platform


def login(message, messagetype):
    """
    the function returns the login page which shows messages
    also the function updates the last commit informations in the config file
    """
    utils = webutils()
    utils.update_last_commit_infos()
    payload = utils.get_default_payload("Signin", "signin")
    if messagetype is not None:
        payload.messagetype = messagetype
    if message is not None:
        payload.message = message
    return render_template('login.html', vars=payload)


def login_post():
    """
    the function handles the login post request and if all information are correct
    a session variable is set to store the login information
    """
    utils = webutils()
    hostname = platform.node().encode("utf-8")
    salt = hashlib.sha512(hostname).hexdigest()
    pw_hash = hashlib.sha512(
        str(salt + request.form['password']).encode("utf-8")).hexdigest()
    if request.form['username'] == utils.cfg.get_value(
            "WEB", "username") and pw_hash == utils.cfg.get_value(
                "WEB", "password"):
        session['logged_in'] = True
        return redirect("/")
    return login("Incorrect username or password.", "danger")


def logout():
    """the function removes the logged_in session variable if the user is logged in"""
    utils = webutils()
    if utils.check_login() is True:
        session['logged_in'] = False
        return redirect("/")
    else:
        return login("", None)
