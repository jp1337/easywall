"""This module exports a generic class for configuration"""
import configparser
import log


class config(object):
    """This class is a wrapper class around configparser"""

    def __init__(self, configpath):
        log.logging.debug("Setting up configuration...")
        self.configpath = configpath
        self.configlib = configparser.ConfigParser()
        log.logging.debug("Reading config file from " + configpath)
        self.configlib.read(self.configpath)
        log.logging.debug("Configuration is set up.")

    def get_value(self, section, key):
        """Returns a value from the configuration file as string or boolean"""
        log.logging.debug(
            "getting configuration value with key " + key + " from section " +
            section)
        if self.configlib[section][key] in ["yes", "no", "true", "false", "on", "off"]:
            return self.configlib.getboolean(section, key)
        return self.configlib[section][key]

    def get_sections(self):
        """Return a list of section names, excluding [DEFAULT]"""
        log.logging.debug("Listing all config sections...")
        return self.configlib.sections()

    def set_value(self, section, key, value):
        """Writes a key, value pair into memory configuration and writes it to config file"""
        value = str(value)
        log.logging.debug(
            "setting configuration value with key " + key + " in section " +
            section + " from " + str(self.get_value(section, key)) + " to " + value)
        self.configlib[section][key] = value
        log.logging.debug("writing configuration file to " + self.configpath)
        with open(self.configpath, 'w') as configfile:
            self.configlib.write(configfile)
