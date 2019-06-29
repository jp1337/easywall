from flask import Flask, render_template, session, redirect, flash, request
from datetime import datetime
import config
import os
import platform
import hashlib

app = Flask(__name__)


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
    cfg = config.config("../config/config.ini")

    if request.form['username'] == cfg.getValue(
            "WEB", "username") and pw_hash == cfg.getValue(
            "WEB", "password"):
        session['logged_in'] = True
    else:
        flash('wrong password!')
    return redirect("/")


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
    if messagetype != None:
        message = ""
    return render_template(
        'login.html', vars=get_default_vars("Signin", "signin"))


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


class DefaultVars(object):
    pass


# only debugging
if __name__ == '__main__':
    app.secret_key = os.urandom(12)
    port = 5000
    host = "0.0.0.0"
    debug = True
    app.run(host, port, debug)

# production mode
if __name__ == 'uwsgi_file_app':
    app.secret_key = os.urandom(12)
