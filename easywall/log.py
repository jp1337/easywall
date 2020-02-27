"""
This is a wrapper around the logging module.
It supports the simple configuration of the outputs.
"""
import logging
from sys import stdout

from easywall.utility import create_folder_if_not_exists


class Log(object):
    """
    This class is the main class of the log module.
    All logging information is required as inputs.
    """

    def __init__(self, loglevel: str, to_stdout: bool, to_files: bool, logpath: str, logfile: str):
        self.loglevel = self.correct_level(loglevel)

        # create logger
        root = logging.getLogger()
        root.handlers.clear()  # workaround for default stdout handler
        root.setLevel(self.loglevel)

        # create formatter and add it to the handlers
        formatter = logging.Formatter(
            '[%(asctime)s] [%(levelname)s] [%(filename)s:%(lineno)d] %(message)s')

        # create console handler -> logs are always written to stdout
        if to_stdout:
            std_handler = logging.StreamHandler(stdout)
            std_handler.setLevel(self.loglevel)
            std_handler.setFormatter(formatter)
            root.addHandler(std_handler)

        # create file handler if enabled in configuration
        if to_files:
            # create log filepath if not exists
            create_folder_if_not_exists(logpath)

            file_handler = logging.FileHandler(
                "{}/{}".format(logpath, logfile))
            file_handler.setLevel(self.loglevel)
            file_handler.setFormatter(formatter)
            root.addHandler(file_handler)

    def close_logging(self):
        """This function gently closes all handlers before exiting the software"""
        root = logging.getLogger()
        for handler in root.handlers:
            handler.close()
            root.removeFilter(handler)

    def correct_level(self, loglevel):
        """This internal function determines the loglevel of the logging class"""
        level = logging.NOTSET
        loglevel = loglevel.lower()
        if loglevel == "debug":
            level = logging.DEBUG
        elif loglevel == "info":
            level = logging.INFO
        elif loglevel == "warning":
            level = logging.WARNING
        elif loglevel == "error":
            level = logging.ERROR
        elif loglevel == "critical":
            level = logging.CRITICAL
        return level
