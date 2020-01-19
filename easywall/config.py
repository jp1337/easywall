"""This module exports a generic class for configuration"""
import configparser

from easywall.utility import is_float, is_int


class Config(object):
    """This class is a wrapper class around configparser"""

    def __init__(self, configpath):
        self.configpath = configpath
        self.configlib = configparser.ConfigParser()
        self.configlib.read(self.configpath)

    def get_value(self, section, key):
        """Returns a value in a given section from the configuration file.
        Returns String, Float, Integer, Boolean"""
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
        return self.configlib.sections()

    def set_value(self, section, key, value):
        """Writes a key, value pair into memory configuration and writes it to config file"""
        value = str(value)
        self.configlib[section][key] = value
        with open(self.configpath, 'w') as configfile:
            self.configlib.write(configfile)
