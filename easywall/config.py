"""This module exports a generic class for configuration"""
import configparser

from easywall.log import logging
from easywall.utility import is_int, is_float


class Config(object):
    """This class is a wrapper class around configparser"""

    def __init__(self, configpath):
        logging.debug("Setting up configuration...")
        self.configpath = configpath
        self.configlib = configparser.ConfigParser()
        logging.debug("Reading config file from {}".format(configpath))
        self.configlib.read(self.configpath)
        logging.debug("Configuration is set up.")

    def get_value(self, section, key):
        """Returns a value in a given section from the configuration file.
        Returns String, Float, Integer, Boolean"""
        logging.debug(
            "searching for key {} in section {}".format(key, section))
        value = self.configlib[section][key]
        if value in ["yes", "no", "true", "false", "on", "off"]:
            return self.configlib.getboolean(section, key)
        if is_int(value):
            return self.configlib.getint(section, key)
        if is_float(value):
            return self.configlib.getfloat(section, key)
        return value

    def get_sections(self):
        """Return a list of section names, excluding [DEFAULT]"""
        logging.debug("List all sections of the configuration file")
        return self.configlib.sections()

    def set_value(self, section, key, value):
        """Writes a key, value pair into memory configuration and writes it to config file"""
        value = str(value)
        logging.debug("setting configuration value '{}' in section '{}' from {} to {}".format(
            key, section, self.get_value(section, key), value))
        self.configlib[section][key] = value
        logging.debug("writing configuration file to {}".format(
            self.configpath))
        with open(self.configpath, 'w') as configfile:
            self.configlib.write(configfile)
