import config
import log
from time import sleep


class acceptance(object):
    def __init__(self):
        self.config = config.config("config/config.ini")
        self.enabled = bool(self.config.getValue("ACCEPTANCE", "enabled"))
        self.filename = self.config.getValue("ACCEPTANCE", "filename")

    def reset(self):
        if self.enabled == True:
            with open(self.filename, 'w') as accfile:
                accfile.write('false')

    def check(self):
        if self.enabled == True:
            seconds = int(self.config.getValue("ACCEPTANCE", "time"))
            log.logging.info(
                "Starting Acceptance Check... waiting for " +
                str(seconds) + " seconds")
            sleep(seconds)
            with open(self.filename, 'r') as accfile:
                accepted = accfile.read()
                accepted = accepted.replace("\n", "")
                if accepted == "true":
                    log.logging.info("Acceptance Process Result: Accepted")
                    return True
                else:
                    log.logging.info(
                        "Acceptance Process Result: Not Accepted (file content: " + accepted + ")")
                    return False
        else:
            return True
