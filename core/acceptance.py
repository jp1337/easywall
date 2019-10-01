"""the module contains a class that is used when accepting the firewall changes"""
from time import sleep

import config
import log


class Acceptance(object):
    """
    the class contains function for checking the user acceptance after applying new firewall rules
    """

    def __init__(self):
        """the init function creates some class variables"""
        self.config = config.Config("config/config.ini")
        self.enabled = bool(self.config.get_value("ACCEPTANCE", "enabled"))
        self.filename = self.config.get_value("ACCEPTANCE", "filename")
        log.logging.debug("Acceptance Process initialized. Status: " +
                          str(self.enabled) + " Filename: " + self.filename)

    def reset(self):
        """the function is called then the user did not accept the changes"""
        if self.enabled:
            with open(self.filename, 'w') as accfile:
                accfile.write('false')
            log.logging.debug("Acceptance has been reset.")

    def check(self):
        """the function checks for acceptance and executes the next steps"""
        if self.enabled:
            seconds = int(self.config.get_value("ACCEPTANCE", "time"))
            log.logging.debug(
                "Starting Acceptance Check... waiting for " + str(seconds) +
                " seconds")
            sleep(seconds)
            with open(self.filename, 'r') as accfile:
                accepted = accfile.read()
                accepted = accepted.replace("\n", "")
                if accepted == "true":
                    log.logging.debug("Acceptance Process Result: Accepted")
                    return True
                else:
                    log.logging.debug(
                        "Acceptance Process Result: Not Accepted (file content: " + accepted + ")")
                    return False
        else:
            log.logging.debug("Acceptance is disabled. Skipping check.")
            return True
