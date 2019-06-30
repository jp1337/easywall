from flask import Flask, render_template, session, redirect, flash, request
from datetime import datetime
import config
import utility
import os
import platform
import hashlib
import urllib
import json

app = Flask(__name__)
cfg = config.config("../config/config.ini")


@app.route('/')
def index():
    if check_login() is True:
        return render_template('index.html', vars=get_default_vars("Home"))
    else:
        return login("", None)


@app.route('/options')
def options():
    if check_login() is True:
        return render_template(
            'options.html', vars=get_default_vars("Options"))
    else:
        return login("", None)


@app.route('/blacklist')
def blacklist():
    if check_login() is True:
        return render_template(
            'blacklist.html', vars=get_default_vars("Blacklist"))
    else:
        return login("", None)


@app.route('/whitelist')
def whitelist():
    if check_login() is True:
        return render_template(
            'whitelist.html', vars=get_default_vars("Whitelist"))
    else:
        return login("", None)


@app.route('/ports')
def ports():
    if check_login() is True:
        return render_template(
            'ports.html', vars=get_default_vars("Ports"))
    else:
        return login("", None)


@app.route('/apply')
def apply():
    if check_login() is True:
        return render_template(
            'apply.html', vars=get_default_vars("Apply"))
    else:
        return login("", None)


@app.route('/login', methods=['POST'])
def login_post():
    hostname = platform.node().encode("utf-8")
    salt = hashlib.sha512(hostname).hexdigest()
    pw_hash = hashlib.sha512(
        str(salt + request.form['password']).encode("utf-8")).hexdigest()

    if request.form['username'] == cfg.getValue(
            "WEB", "username") and pw_hash == cfg.getValue(
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
            '404.html', vars=get_default_vars("404 Error", "error")), 404
    else:
        return login("", None)


def login(message, messagetype):
    vars = get_default_vars("Signin", "signin")
    if messagetype != None:
        vars.messagetype = messagetype
    if message != None:
        vars.message = message
    return render_template('login.html', vars=vars)


def check_login():
    if not session.get('logged_in'):
        return False
    return True


def get_default_vars(title, css="easywall"):
    vars = DefaultVars()
    vars.year = datetime.today().year
    vars.title = title
    vars.customcss = css
    vars.machine = get_machine_infos()
    vars.latest_version = get_latest_version()
    vars.current_version = utility.file_get_contents("../.version")
    vars.commit_sha = get_latest_commit()["sha"]
    vars.commit_date = get_commit_date(
        get_latest_commit()["commit"]["author"]["date"])
    return vars


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
    d["Linux Distribution"] = platform.linux_distribution()
    d["Libc Version"] = platform.libc_ver()
    return d


def get_latest_version():
    url = "https://raw.githubusercontent.com/jp1337/easywall/master/.version"
    response = urllib.request.urlopen(url)
    data = response.read()
    return data.decode('utf-8')


def get_latest_commit():
    url = "https://api.github.com/repos/jp1337/easywall/commits/master"
    req = urllib.request.Request(
        url,
        data=None,
        headers={
            'User-Agent': 'EasyWall Firewall by jp1337',
            'Authorization': 'token 87bfd3cabedccabd8e4f829ff9b76e2470f2aee7'
        }
    )
    response = urllib.request.urlopen(req)
    return json.loads(response.read().decode('utf-8'))


def get_commit_date(datestring):
    d1 = datetime.strptime(datestring, "%Y-%m-%dT%H:%M:%SZ")
    d2 = datetime.now()
    return utility.time_duration_diff(d1, d2)


class DefaultVars(object):
    pass


# only debugging
if __name__ == '__main__':
    app.secret_key = os.urandom(12)
    port = int(cfg.getValue("WEB", "bindport"))
    host = cfg.getValue("WEB", "bindip")
    debug = True
    app.run(host, port, debug)

# production mode
if __name__ == 'uwsgi_file_app':
    app.secret_key = os.urandom(12)
