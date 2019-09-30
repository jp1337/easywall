"""the app module contains all information of the Flask app"""
import hashlib
import json
import os
import platform
import time
import urllib
from datetime import datetime, timezone

from flask import Flask, redirect, render_template, request, session

import config
import utility

APP = Flask(__name__)
CFG = config.Config("../config/config.ini")


@APP.route('/')
def index():
    """the function returns the index page when the user is logged in"""
    if check_login() is True:
        return render_template('index.html', vars=get_default_payload("Home"))
    return login("", None)


@APP.route('/options')
def options(saved=False):
    """the function returns the options page when the user is logged in"""
    if check_login() is True:
        payload = get_default_payload("Options")
        payload.config = CFG
        payload.saved = saved
        return render_template(
            'options.html', vars=payload)
    return login("", None)


@APP.route('/blacklist')
def blacklist(saved=False):
    """the function returns the blacklist page when the user is logged in"""
    if check_login() is True:
        payload = get_default_payload("Blacklist")
        payload.addresses = get_rule_list("blacklist")
        payload.saved = saved
        return render_template(
            'blacklist.html', vars=payload)
    return login("", None)


@APP.route('/whitelist')
def whitelist(saved=False):
    """the function returns the whitelist page when the user is logged in"""
    if check_login() is True:
        payload = get_default_payload("Whitelist")
        payload.addresses = get_rule_list("whitelist")
        payload.saved = saved
        return render_template(
            'whitelist.html', vars=payload)
    return login("", None)


@APP.route('/ports')
def ports(saved=False):
    """the function returns the ports page when the user is logged in"""
    if check_login() is True:
        payload = get_default_payload("Ports")
        payload.tcp = get_rule_list("tcp")
        payload.udp = get_rule_list("udp")
        payload.saved = saved
        return render_template(
            'ports.html', vars=payload)
    return login("", None)


@APP.route('/custom')
def custom(saved=False):
    """the function returns the custom rules page when the user is logged in"""
    if check_login() is True:
        payload = get_default_payload("Custom")
        payload.custom = get_rule_list("custom")
        payload.saved = saved
        return render_template(
            'custom.html', vars=payload)
    return login("", None)


@APP.route('/apply')
def apply(saved=False, step=1):
    """the function returns the apply page when the user is logged in"""
    if check_login() is True:
        payload = get_default_payload("Apply")
        payload.saved = saved
        payload.step = step
        payload.lastapplied = get_last_accept_time()
        payload.accepttime = CFG.get_value("ACCEPTANCE", "time")
        return render_template(
            'apply.html', vars=payload)
    return login("", None)


@APP.route('/options-save', methods=['POST'])
def options_save():
    """
    the function saves the options from a section using the config class
    for example the Enabled flag in the IPv6 section is saved to the config file
    """
    if check_login() is True:
        section = request.form['section']
        for key, value in request.form.items():
            if key != "section":
                # checkbox workaround.
                if key.startswith("checkbox"):
                    key = key.replace("checkbox_", "")
                    if key in request.form:
                        value = "yes"
                    else:
                        value = "no"
                if value != "on":
                    CFG.set_value(section, key, value)
        return options(True)
    return login("", None)


@APP.route('/blacklist-save', methods=['POST'])
def blacklist_save():
    """the function saves the blacklist rules into the corresponding rulesfile"""
    if check_login() is True:
        ipaddress = ""
        rulelist = get_rule_list("blacklist")

        for key, value in request.form.items():
            if key == "ipadr":
                # then a new ip address is blacklisted
                ipaddress = value
                rulelist.append(ipaddress)
                save_rule_list("blacklist", rulelist)
            else:
                # then a old ip address is removed
                ipaddress = key
                rulelist.remove(ipaddress)
                save_rule_list("blacklist", rulelist)
        return blacklist(True)
    return login("", None)


@APP.route('/whitelist-save', methods=['POST'])
def whitelist_save():
    """the function saves the whitelist rules into the corresponding rulesfile"""
    if check_login() is True:
        ipaddress = ""
        rulelist = get_rule_list("whitelist")

        for key, value in request.form.items():
            if key == "ipadr":
                # then a new ip address is whitelisted
                ipaddress = value
                rulelist.append(ipaddress)
                save_rule_list("whitelist", rulelist)
            else:
                # then a old ip address is removed
                ipaddress = key
                rulelist.remove(ipaddress)
                save_rule_list("whitelist", rulelist)
        return whitelist(True)
    return login("", None)


@APP.route('/ports-save', methods=['POST'])
def ports_save():
    """the function saves the tcp and udp rules into the corresponding rulesfiles"""
    if check_login() is True:
        tcp = get_rule_list("tcp")
        udp = get_rule_list("udp")
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
                save_rule_list("tcp", tcp)
            else:
                udp.append(port)
                save_rule_list("udp", udp)
        else:
            if ruletype == "tcp":
                tcp.remove(port)
                save_rule_list("tcp", tcp)
            else:
                udp.remove(port)
                save_rule_list("udp", udp)

        return ports(True)
    return login("", None)


@APP.route('/custom-save', methods=['POST'])
def custom_save():
    """the function saves the custom rules into the corresponding rulesfile"""
    if check_login() is True:
        for key, value in request.form.items():
            rulelist = value.split("\n")
            save_rule_list("custom", rulelist)
        return custom(True)
    return login("", None)


@APP.route('/login', methods=['POST'])
def login_post():
    """
    the function handles the login post request and if all information are correct
    a session variable is set to store the login information
    """
    hostname = platform.node().encode("utf-8")
    salt = hashlib.sha512(hostname).hexdigest()
    pw_hash = hashlib.sha512(
        str(salt + request.form['password']).encode("utf-8")).hexdigest()
    if request.form['username'] == CFG.get_value(
            "WEB", "username") and pw_hash == CFG.get_value(
                "WEB", "password"):
        session['logged_in'] = True
        return redirect("/")
    return login("Incorrect username or password.", "danger")


@APP.route("/logout")
def logout():
    """the function removes the logged_in session variable if the user is logged in"""
    if check_login() is True:
        session['logged_in'] = False
        return redirect("/")
    else:
        return login("", None)


@APP.errorhandler(404)
def page_not_found(error):
    """the function returns the 404 error page when the user is logged in"""
    if check_login() is True:
        return render_template(
            '404.html', vars=get_default_payload("404 Error", "error")), 404
    return login("", None)


def login(message, messagetype):
    """
    the function returns the login page which shows messages
    also the function updates the last commit informations in the config file
    """
    update_last_commit_infos()
    payload = get_default_payload("Signin", "signin")
    if messagetype is not None:
        payload.messagetype = messagetype
    if message is not None:
        payload.message = message
    return render_template('login.html', vars=payload)


def check_login():
    """the function checks if the user/session is logged in"""
    if not session.get('logged_in'):
        return False
    return True


def get_default_payload(title, css="easywall"):
    """the function creates a object of information that are needed on every page"""
    payload = DefaultPayload()
    payload.year = datetime.today().year
    payload.title = title
    payload.customcss = css
    payload.machine = get_machine_infos()
    payload.latest_version = CFG.get_value("VERSION", "version")
    payload.current_version = utility.file_get_contents("../.version")
    payload.commit_sha = CFG.get_value("VERSION", "sha")
    payload.commit_date = get_commit_date(CFG.get_value("VERSION", "date"))
    return payload


def get_machine_infos():
    """the function retrieves some information about the host and returns them as a list"""
    infos = {}
    infos["Machine"] = platform.machine()
    infos["Hostname"] = platform.node()
    infos["Platform"] = platform.platform()
    infos["Python Build"] = platform.python_build()
    infos["Python Compiler"] = platform.python_compiler()
    infos["Python Implementation"] = platform.python_implementation()
    infos["Python Version"] = platform.python_version()
    infos["Release"] = platform.release()
    infos["Libc Version"] = platform.libc_ver()
    return infos


def get_latest_version():
    """
    the function retrieves the latest version from github and returns the version string
    """
    url = "https://raw.githubusercontent.com/jp1337/easywall/master/.version"
    req = urllib.request.Request(
        url,
        data=None,
        headers={
            'User-Agent': 'EasyWall Firewall by jp1337'
        }
    )
    response = urllib.request.urlopen(req)
    data = response.read()
    return data.decode('utf-8')


def update_last_commit_infos():
    """
    the function retrieves the last commit information after a specific waiting time
    after retrieving the information they are saved into the config file
    """
    currtime = int(time.time())
    lasttime = int(CFG.get_value("VERSION", "timestamp"))
    waitseconds = 3600  # 60 minutes × 60 seconds
    if currtime > (lasttime + waitseconds):
        commit = get_latest_commit()
        CFG.set_value("VERSION", "version", get_latest_version())
        CFG.set_value("VERSION", "sha", commit["sha"])
        CFG.set_value("VERSION", "date", commit["commit"]["author"]["date"])
        CFG.set_value("VERSION", "timestamp", currtime)


def get_latest_commit():
    """
    retrieves the informations of the last commit from github as json
    and converts the information into a python object
    for example the object contains the last commit date and the last commit sha
    This function should not be called very often, because GitHub has a rate limit implemented
    """
    url = "https://api.github.com/repos/jp1337/easywall/commits/master"
    req = urllib.request.Request(
        url,
        data=None,
        headers={
            'User-Agent': 'EasyWall Firewall by jp1337'
        }
    )
    response = urllib.request.urlopen(req)
    return json.loads(response.read().decode('utf-8'))


def get_commit_date(datestring):
    """
    the function compares a datetime with the current date
    for comparing the datestring parameter is in UTC timezone
    """
    date1 = datetime.strptime(str(datestring), "%Y-%m-%dT%H:%M:%SZ")
    date1 = date1.replace(
        tzinfo=timezone.utc).astimezone(
            tz=None).replace(
                tzinfo=None)
    date2 = datetime.now()
    return utility.time_duration_diff(date1, date2)


def get_rule_list(ruletype):
    """
    the function reads a file into the ram and returns a list of all rows in a list
    for example you get all the ip addresses of the blacklist in a array
    """
    rule_list = []
    filepath = CFG.get_value(
        "RULES", "filepath") + "/" + CFG.get_value("RULES", ruletype)
    if filepath.startswith("."):
        filepath = "../" + filepath
    with open(filepath, 'r') as rulesfile:
        for rule in rulesfile.read().split('\n'):
            if rule.strip() != "":
                rule_list.append(rule)
    return rule_list


def save_rule_list(ruletype, rulelist):
    """
    the function writes a list of strings into a rulesfile
    for example it saves the blacklist rules into the blacklist rulesfile
    """
    filepath = CFG.get_value(
        "RULES", "filepath") + "/" + CFG.get_value("RULES", ruletype)
    if filepath.startswith("."):
        filepath = "../" + filepath
    with open(filepath, mode='wt', encoding='utf-8') as rulesfile:
        rulesfile.write('\n'.join(rulelist))
    return True


def get_last_accept_time():
    """
    the function retrieves the modify time of the acceptance file and compares the time to the current time
    """
    filepath = "../" + CFG.get_value("ACCEPTANCE", "filename")
    if os.path.exists(filepath):
        mtime = os.path.getmtime(filepath)
        mtime = datetime.utcfromtimestamp(mtime)
        mtime = mtime.replace(
            tzinfo=timezone.utc).astimezone(
            tz=None).replace(
                tzinfo=None)
        now = datetime.now()
        return utility.time_duration_diff(mtime, now)
    else:
        return "never"


class DefaultPayload(object):
    """the class is a empty skeleton for generating objects"""

    def __init__(self):
        self.config = None
        self.saved = None
        self.addresses = None
        self.tcp = None
        self.udp = None
        self.messagetype = None
        self.message = None
        self.year = None
        self.title = None
        self.customcss = None
        self.machine = None
        self.latest_version = None
        self.current_version = None
        self.commit_sha = None
        self.commit_date = None


# only debugging
if __name__ == '__main__':
    APP.secret_key = os.urandom(12)
    PORT = int(CFG.get_value("WEB", "bindport"))
    HOST = CFG.get_value("WEB", "bindip")
    DEBUG = True
    APP.run(HOST, PORT, DEBUG)

# production mode
if __name__ == 'uwsgi_file_app':
    APP.secret_key = os.urandom(12)
