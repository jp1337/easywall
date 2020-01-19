"""the module contains a class that is used when accepting the firewall changes"""
from time import sleep

from easywall.config import Config
from easywall.log import logging
from easywall.utility import create_file_if_not_exists, write_into_file


class Acceptance(object):
    """
    the class contains function for checking the user acceptance after applying new firewall rules
    """

    def __init__(self):
        """the init function creates some class variables"""
        self.config = Config("config/easywall.ini")
        self.enabled = bool(self.config.get_value("ACCEPTANCE", "enabled"))
        self.filename = self.config.get_value("ACCEPTANCE", "filename")
        logging.debug("Acceptance Process initialized. Status: " +
                      str(self.enabled) + " Filename: " + self.filename)

    def reset(self):
        """the function is called then the user did not accept the changes"""
        if self.enabled:
            create_file_if_not_exists(self.filename)
            write_into_file(self.filename, "false")
            logging.debug("Acceptance has been reset.")

    def check(self):
        """the function checks for acceptance and executes the next steps"""
        if self.enabled:
            seconds = int(self.config.get_value("ACCEPTANCE", "time"))
            logging.debug(
                "Starting Acceptance Check... waiting for " + str(seconds) +
                " seconds")
            while seconds > 0:
                sleep(1)
                seconds = seconds - 1
            with open(self.filename, 'r') as accfile:
                accepted = accfile.read()
                accepted = accepted.replace("\n", "")
                if accepted == "true":
                    logging.debug("Acceptance Process Result: Accepted")
                    return True
                else:
                    logging.debug(
                        "Acceptance Process Result: Not Accepted (file content: " + accepted + ")")
                    return False
        else:
            logging.debug("Acceptance is disabled. Skipping check.")
            return True
