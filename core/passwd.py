import config
import getpass
import hashlib
import platform


class passwd(object):

    def __init__(self):
        self.config = config.config("config/config.ini")
        self.ask_user()

    def savepasswd(self, password):
        hostname = platform.node().encode("utf-8")
        salt = hashlib.sha512(hostname).hexdigest()
        pw_hash = hashlib.sha512(
            str(salt + password).encode("utf-8")).hexdigest()
        self.config.set_value("WEB", "password", pw_hash)
        print("Password successfully saved.")

    def saveuser(self, username):
        self.config.set_value("WEB", "username", username)
        print("Username successfully saved.")

    def ask_user(self):
        username = input("EasyWall Web Username: ")
        self.saveuser(username)
        password = getpass.getpass("EasyWall Web Password: ")
        self.savepasswd(password)


passwd = passwd()
