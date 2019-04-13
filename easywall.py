import os
import configparser
import logging


class iptables(object):
    def __init__(self):
        self.config = config("config/config.ini")
        self.iptables = self.config.getValue("EXEC", "IPTables")
        self.ip6tables = self.config.getValue("EXEC", "IP6Tables")

    def reset(self):
        log().info("Beginning to reset the firewall rules and opening the firewall for every connection")

        os.system(self.iptables + " -P INPUT ACCEPT")
        os.system(self.iptables + " -P OUTPUT ACCEPT")
        os.system(self.iptables + " -P FORWARD ACCEPT")
        os.system(self.iptables + " -F")
        os.system(self.iptables + " -X")
        os.system(self.iptables + " -t nat -F")
        os.system(self.iptables + " -t nat -X")
        os.system(self.iptables + " -t mangle -F")
        os.system(self.iptables + " -t mangle -X")

        os.system(self.ip6tables + " -P INPUT ACCEPT")
        os.system(self.ip6tables + " -P OUTPUT ACCEPT")
        os.system(self.ip6tables + " -P FORWARD ACCEPT")
        os.system(self.ip6tables + " -F")
        os.system(self.ip6tables + " -X")
        os.system(self.ip6tables + " -t nat -F")
        os.system(self.ip6tables + " -t nat -X")
        os.system(self.ip6tables + " -t mangle -F")
        os.system(self.ip6tables + " -t mangle -X")

        log().info("The firewall rules for IPV4 and IPV6 have been successfully reset")

    # def addrule(self, amount):
        # self.balance -= amount


class config(object):

    def __init__(self, configpath):
        self.configpath = configpath
        self.config = configparser.ConfigParser()
        self.config.read(self.configpath)

    def getValue(self, section, key):
        return self.config[section][key]

    def getSections(self):
        return self.getSections()

    def setValue(self, section, key, value):
        self.config[section][key] = value
        with open(self.configpath, 'w') as configfile:
            self.config.write(configfile)


class log(object):
    def __init__(self):
        self.config = config("config/config.ini")
        logging.basicConfig(
            filename=self.config.getValue("LOG", "LogFile"),
            level=logging.DEBUG,
            format='%(asctime)s %(message)s',
            datefmt='%m/%d/%Y %I:%M:%S %p'
        )

    def info(self, message):
        logging.info(message)

    def warning(self, message):
        logging.warning(message)

    def error(self, message):
        logging.error(message)


class easywall(object):
    def __init__(self):
        self.ipt = iptables()
        self.ipt.reset()


easywall()
