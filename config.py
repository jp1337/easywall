import configparser
import log


class config(object):

    def __init__(self, configpath):
        log.logging.info("Setting up logging...")
        self.configpath = configpath
        self.config = configparser.ConfigParser()
        log.logging.info("Reading config file from " + configpath)
        self.config.read(self.configpath)
        log.logging.info("Configuration is set up.")

    def getValue(self, section, key):
        log.logging.debug(
            "getting configuration value with key " + key + " from section " + section)
        return self.config[section][key]

    def getSections(self):
        log.logging.debug("Listing all config sections...")
        return self.getSections()

    def setValue(self, section, key, value):
        log.logging.debug("setting configuration value with key " + key + " in section " +
                          section + " from " + self.getValue(section, key) + " to " + value)
        self.config[section][key] = value
        log.logging.debug("writing configuration file to " + self.configpath)
        with open(self.configpath, 'w') as configfile:
            self.config.write(configfile)
