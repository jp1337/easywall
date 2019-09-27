from flask import Flask, render_template, session, redirect, request
from datetime import datetime, timezone
import config
import utility
import os
import platform
import hashlib
import urllib
import json
import time

app = Flask(__name__)
cfg = config.Config("../config/config.ini")


@app.route('/')
def index():
    if check_login() is True:
        return render_template('index.html', vars=get_default_payload("Home"))
    else:
        return login("", None)


@app.route('/options')
def options(saved=False):
    if check_login() is True:
        payload = get_default_payload("Options")
        payload.config = cfg
        payload.saved = saved
        return render_template(
            'options.html', vars=payload)
    else:
        return login("", None)


@app.route('/blacklist')
def blacklist():
    if check_login() is True:
        payload = get_default_payload("Blacklist")
        payload.addresses = get_rule_list("blacklist")
        return render_template(
            'blacklist.html', vars=payload)
    else:
        return login("", None)


@app.route('/whitelist')
def whitelist():
    if check_login() is True:
        return render_template(
            'whitelist.html', vars=get_default_payload("Whitelist"))
    else:
        return login("", None)


@app.route('/ports')
def ports():
    if check_login() is True:
        return render_template(
            'ports.html', vars=get_default_payload("Ports"))
    else:
        return login("", None)


@app.route('/custom')
def custom():
    if check_login() is True:
        return render_template(
            'custom.html', vars=get_default_payload("Custom"))
    else:
        return login("", None)


@app.route('/apply')
def apply():
    if check_login() is True:
        return render_template(
            'apply.html', vars=get_default_payload("Apply"))
    else:
        return login("", None)


@app.route('/options-save', methods=['POST'])
def options_save():
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
                    cfg.set_value(section, key, value)
        return options(True)
    else:
        return login("", None)


@app.route('/login', methods=['POST'])
def login_post():
    hostname = platform.node().encode("utf-8")
    salt = hashlib.sha512(hostname).hexdigest()
    pw_hash = hashlib.sha512(
        str(salt + request.form['password']).encode("utf-8")).hexdigest()

    if request.form['username'] == cfg.get_value(
            "WEB", "username") and pw_hash == cfg.get_value(
            "WEB", "password"):
        session['logged_in'] = True
        return redirect("/")
    return login("Incorrect username or password.", "danger")


@app.route("/logout")
def logout():
    if check_login() is True:
        session['logged_in'] = False
        return redirect("/")
    else:
        return login("", None)


@app.errorhandler(404)
def page_not_found(e):
    if check_login() is True:
        return render_template(
            '404.html', vars=get_default_payload("404 Error", "error")), 404
    else:
        return login("", None)


def login(message, messagetype):
    update_last_commit_infos()
    payload = get_default_payload("Signin", "signin")
    if messagetype != None:
        payload.messagetype = messagetype
    if message != None:
        payload.message = message
    return render_template('login.html', vars=payload)


def check_login():
    if not session.get('logged_in'):
        return False
    return True


def get_default_payload(title, css="easywall"):
    payload = DefaultPayload()
    payload.year = datetime.today().year
    payload.title = title
    payload.customcss = css
    payload.machine = get_machine_infos()
    payload.latest_version = cfg.get_value("VERSION", "version")
    payload.current_version = utility.file_get_contents("../.version")
    payload.commit_sha = cfg.get_value("VERSION", "sha")
    payload.commit_date = get_commit_date(cfg.get_value("VERSION", "date"))
    return payload


def get_machine_infos():
    d = {}
    d["Machine"] = platform.machine()
    d["Hostname"] = platform.node()
    d["Platform"] = platform.platform()
    d["Python Build"] = platform.python_build()
    d["Python Compiler"] = platform.python_compiler()
    d["Python Implementation"] = platform.python_implementation()
    d["Python Version"] = platform.python_version()
    d["Release"] = platform.release()
    d["Libc Version"] = platform.libc_ver()
    return d


def get_latest_version():
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
    currtime = int(time.time())
    lasttime = int(cfg.get_value("VERSION", "timestamp"))
    waitseconds = 3600  # 60 minutes × 60 seconds
    if (currtime > lasttime + waitseconds):
        commit = get_latest_commit()
        cfg.set_value("VERSION", "version", get_latest_version())
        cfg.set_value("VERSION", "sha", commit["sha"])
        cfg.set_value("VERSION", "date", commit["commit"]["author"]["date"])
        cfg.set_value("VERSION", "timestamp", currtime)


def get_latest_commit():
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
    d1 = datetime.strptime(str(datestring), "%Y-%m-%dT%H:%M:%SZ")
    d1 = d1.replace(
        tzinfo=timezone.utc).astimezone(
        tz=None).replace(
        tzinfo=None)
    d2 = datetime.now()
    return utility.time_duration_diff(d1, d2)


def get_rule_list(ruletype):
    rule_list = []
    filepath = cfg.get_value(
        "RULES", "filepath") + "/" + cfg.get_value("RULES", ruletype)
    if filepath.startswith("."):
        filepath = "../" + filepath
    with open(filepath, 'r') as rulesfile:
        for rule in rulesfile.read().split('\n'):
            if rule.strip() != "":
                rule_list.append(rule)
    return rule_list


class DefaultPayload(object):
    pass


# only debugging
if __name__ == '__main__':
    app.secret_key = os.urandom(12)
    port = int(cfg.get_value("WEB", "bindport"))
    host = cfg.get_value("WEB", "bindip")
    debug = True
    app.run(host, port, debug)

# production mode
if __name__ == 'uwsgi_file_app':
    app.secret_key = os.urandom(12)
