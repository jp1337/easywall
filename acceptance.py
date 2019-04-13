import config
import log
from time import sleep


class acceptance(object):
    def __init__(self):
        self.config = config.config("config/config.ini")
        self.filename = self.config.getValue("ACCEPTANCE", "filename")

    def reset(self):
        with open(self.filename, 'w') as accfile:
            accfile.write('false')

    def check(self):
        seconds = self.config.getValue("ACCEPTANCE", "time")
        log.logging.info(
            "Starting Acceptance Check... waiting for " + seconds + " seconds")
        sleep(int(seconds))
        with open(self.filename, 'r') as accfile:
            accepted = accfile.read()
            if accepted == "true":
                log.logging.info("Acceptance Process Result: Accepted")
                return True
            else:
                log.logging.info(
                    "Acceptance Process Result: Not Accepted (file content: " + accepted + ")")
                return False
