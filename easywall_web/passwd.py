"""the module creates a new password and writes the password into the config file"""
import getpass
import hashlib
import platform

from easywall.config import Config


class Passwd(object):
    """the class contains the password generation and saving"""

    def __init__(self):
        """the init function creates the config variable and calls the user input"""
        self.config = Config("config/web.ini")
        self.ask_user()

    def savepasswd(self, password):
        """the function saves the password into the config file using the config class"""
        hostname = platform.node().encode("utf-8")
        salt = hashlib.sha512(hostname).hexdigest()
        pw_hash = hashlib.sha512(
            str(salt + password).encode("utf-8")).hexdigest()
        self.config.set_value("WEB", "password", pw_hash)
        print("Password successfully saved.")

    def saveuser(self, username):
        """the function saves the username into the config file using the config class"""
        self.config.set_value("WEB", "username", username)
        print("Username successfully saved.")

    def ask_user(self):
        """the function asks the user for the username and password"""
        username = input("easywall Web Username: ")
        self.saveuser(username)
        password = getpass.getpass("easywall Web Password: ")
        self.savepasswd(password)


if __name__ == "__main__":
    PASSWD = Passwd()
