"""This module exports a generic class for configuration"""
from configparser import ConfigParser
from logging import error, info

from easywall.utility import is_float, is_int


class Config(object):
    """This class is a wrapper class around configparser"""

    def __init__(self, configpath: str):
        self.configpath = configpath
        self.configlib = ConfigParser()
        self.configlib.read(self.configpath)

    def get_value(self, section: str, key: str):
        """Returns a value in a given section from the configuration file.
        Returns String, Float, Integer, Boolean"""
        value = ""
        try:
            value = self.configlib[section][key]
        except KeyError:
            error("Could not find key {} in section {}".format(key, section))
            info("Valid sections are: ")
            info("{}".format(self.get_sections()))
        if value in ["yes", "no", "true", "false", "on", "off"]:
            return self.configlib.getboolean(section, key)
        if is_int(value):
            return self.configlib.getint(section, key)
        if is_float(value):
            return self.configlib.getfloat(section, key)
        return value

    def get_sections(self):
        """Return a list of section names, excluding [DEFAULT]"""
        sections = ""
        try:
            sections = self.configlib.sections()
        except Exception as exc:
            error("Error while reading sections from config file: {}".format(exc))
        return sections

    def set_value(self, section: str, key: str, value: str):
        """Writes a key, value pair into memory configuration and writes it to config file"""
        result = True
        try:
            self.configlib[section][key] = value
        except Exception as exc:
            error(
                "Error while writing {} into key {} in section {}: {}".format(
                    value, key, section, exc))
            info("Valid sections are: ")
            info("{}".format(self.get_sections()))
            result = False

        try:
            with open(self.configpath, 'w') as configfile:
                self.configlib.write(configfile)
        except Exception as exc:
            error(
                "Error while writing configuration into file {}: {}".format(
                    self.configpath, exc))
            result = False

        return result
