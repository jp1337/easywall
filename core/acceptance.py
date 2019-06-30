import config
import log
from time import sleep


class acceptance(object):
    def __init__(self):
        self.config = config.config("config/config.ini")
        self.enabled = bool(self.config.get_value("ACCEPTANCE", "enabled"))
        self.filename = self.config.get_value("ACCEPTANCE", "filename")
        log.logging.debug("Acceptance Process initialized. Status: " +
                          str(self.enabled) + " Filename: " + self.filename)

    def reset(self):
        if self.enabled == True:
            with open(self.filename, 'w') as accfile:
                accfile.write('false')
            log.logging.debug("Acceptance has been reset.")

    def check(self):
        if self.enabled == True:
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
