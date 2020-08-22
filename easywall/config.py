"""
This module exports a generic class for configuration.

[Classes] Config
"""
from configparser import ParsingError, RawConfigParser
from logging import error, info
from typing import AbstractSet, Union

from easywall.utility import file_exists, format_exception, is_float, is_int


class Config():
    """
    This class is a generic class for configuration.

    It is a wrapper around the default configparser and contains basic functionality.

    [Methods]
    get_value: retrieve a value from a config file
    set_value: set a value in the configuration and write the config file to disk
    get_sections: get a list of all possible config sections

    [Raises]
    FileNotFoundError: When the configuration file was not found a exception is thrown.
    Exception: when the configparser failed to read the config file a exception is thrown.
    """

    def __init__(self, config_file_path: str) -> None:
        """TODO: Doku."""
        self.config_file_path = config_file_path
        self.configlib = RawConfigParser()
        self.read_config_file()

    def read_config_file(self) -> None:
        """TODO: Doku."""
        if not file_exists(self.config_file_path):
            raise FileNotFoundError("config file '{}' not found".format(self.config_file_path))
        try:
            self.configlib.read(self.config_file_path)
        except ParsingError as exc:
            raise ParsingError(
                "{} is not readable by RawConfigParser. \n inner exception: {}".format(
                    self.config_file_path, format_exception(exc)))

    def get_value(self, section: str, key: str) -> Union[bool, int, float, str]:
        """
        Return a value from a given section of the configuration.

        [Data Types] String, Float, Integer, Boolean
        """
        self.read_config_file()
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

    def set_value(self, section: str, key: str, value: str) -> bool:
        """
        Write a key, value pair into memory configuration and writes it to config file.

        [Data Types] bool
        """
        result = True
        try:
            self.configlib[section][key] = value
        except KeyError as exc:
            message = "Failed to write data to configuration: \n " + \
                "section: '{}' \n key: '{}' \n value: '{}' \n " + \
                "valid sections are: \n {} \n inner error: \n {}"
            error(message.format(section, key, value, self.get_sections(), format_exception(exc)))
            result = False

        if result:
            with open(self.config_file_path, 'w') as configfile:
                self.configlib.write(configfile)

        return result

    def get_sections(self) -> list:
        """
        Return a list of the configuration section names/keys.

        [WARNING] The name [DEFAULT] is excluded here if used!

        [Data Types] list
        """
        return self.configlib.sections()

    def get_keys(self, section: str) -> AbstractSet[str]:
        """TODO: Doku."""
        return self.configlib[section].keys()
