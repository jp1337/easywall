"""the module creates a new password and writes the password into the config file"""
import getpass
import hashlib
import platform
import argparse

from easywall.config import Config
from easywall.web.__main__ import CONFIG_PATH


class Passwd(object):
    """the class contains the password generation and saving"""

    def __init__(self) -> None:
        """the init function creates the config variable and calls the user input"""
        self.config = Config(CONFIG_PATH)

        parser = argparse.ArgumentParser()
        parser.add_argument("--username", "-u", help="set username")
        parser.add_argument("--password", "-p", help="set password")
        args = parser.parse_args()

        if args.username and args.password:
            self.saveuser(args.username)
            self.savepasswd(args.password)
        else:
            self.ask_user()

    def savepasswd(self, password: str) -> None:
        """the function saves the password into the config file using the config class"""
        hostname = platform.node().encode("utf-8")
        salt = hashlib.sha512(hostname).hexdigest()
        pw_hash = hashlib.sha512(
            str(salt + password).encode("utf-8")).hexdigest()
        self.config.set_value("WEB", "password", pw_hash)
        print("Password successfully saved.")

    def saveuser(self, username: str) -> None:
        """the function saves the username into the config file using the config class"""
        self.config.set_value("WEB", "username", username)
        print("Username successfully saved.")

    def ask_user(self) -> None:
        """the function asks the user for the username and password"""
        username = input("easywall Web Username: ")
        self.saveuser(username)
        password = getpass.getpass("easywall Web Password: ")
        password_repeat = getpass.getpass("easywall Web Password repeat: ")
        if password == password_repeat:
            self.savepasswd(password)
        else:
            print("password is not equal. password not saved!")


if __name__ == "__main__":
    PASSWD = Passwd()
