"""This module exports a generic class for Easier Logging"""
import logging
from sys import stdout

from easywall.config import Config
from easywall.utility import create_folder_if_not_exists


class Log(object):
    """This class is a wrapper class around the logging module"""

    def __init__(self, configpath):
        self.config = Config(configpath)
        self.loglevel = self.get_level(self.config.get_value("LOG", "level"))

        # create logger
        root = logging.getLogger()
        root.handlers.clear()  # workaround for default stdout handler
        root.setLevel(self.loglevel)

        # create formatter and add it to the handlers
        formatter = logging.Formatter(
            '[%(asctime)s] [%(levelname)s] [%(filename)s:%(lineno)d] %(message)s')

        # create console handler -> logs are always written to stdout
        if bool(self.config.get_value("LOG", "to_stdout")):
            std_handler = logging.StreamHandler(stdout)
            std_handler.setLevel(self.loglevel)
            std_handler.setFormatter(formatter)
            root.addHandler(std_handler)

        # create file handler if enabled in configuration
        if bool(self.config.get_value("LOG", "to_files")):
            # create log filepath if not exists
            create_folder_if_not_exists(
                self.config.get_value("LOG", "filepath"))

            file_handler = logging.FileHandler(
                self.config.get_value("LOG", "filepath") + "/" +
                self.config.get_value("LOG", "filename")
            )
            file_handler.setLevel(self.loglevel)
            file_handler.setFormatter(formatter)
            root.addHandler(file_handler)

    def close_logging(self):
        """This function gently closes all handlers before exiting the software"""
        root = logging.getLogger()
        for handler in root.handlers:
            handler.close()
            root.removeFilter(handler)

    def get_level(self, log_level):
        """This internal function determines the log_level of the logging class"""
        level = logging.NOTSET
        if log_level == "debug":
            level = logging.DEBUG
        elif log_level == "info":
            level = logging.INFO
        elif log_level == "warning":
            level = logging.WARNING
        elif log_level == "error":
            level = logging.ERROR
        elif log_level == "critical":
            level = logging.CRITICAL
        return level
