import configparser
import log


class config(object):

    def __init__(self, configpath):
        log.logging.debug("Setting up configuration...")
        self.configpath = configpath
        self.configlib = configparser.ConfigParser()
        log.logging.debug("Reading config file from " + configpath)
        self.configlib.read(self.configpath)
        log.logging.debug("Configuration is set up.")

    def getValue(self, section, key):
        log.logging.debug(
            "getting configuration value with key " + key + " from section " + section)
        if self.configlib[section][key] in ["yes", "no", "true", "false", "on", "off"]:
            return self.configlib.getboolean(section, key)
        return self.configlib[section][key]

    def getSections(self):
        log.logging.debug("Listing all config sections...")
        return self.getSections()

    def setValue(self, section, key, value):
        log.logging.debug("setting configuration value with key " + key + " in section " +
                          section + " from " + self.getValue(section, key) + " to " + value)
        self.configlib[section][key] = value
        log.logging.debug("writing configuration file to " + self.configpath)
        with open(self.configpath, 'w') as configfile:
            self.configlib.write(configfile)
