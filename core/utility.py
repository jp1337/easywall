"""This file contains useful functions which should be used instead of implementing in a class"""
import csv
import math
import os
import stat
import urllib

import log

# -------------------------
# File Operations


def create_folder_if_not_exists(filepath: str):
    """Checks if a folder exists and creates if it does not exist"""
    if not os.path.exists(filepath):
        log.logging.debug("creating folder: {}".format(filepath))
        os.makedirs(filepath)


def create_file_if_not_exists(fullpath: str):
    """The function creates a file if it does not already exist."""
    if not os.path.isfile(fullpath) or not os.access(fullpath, os.R_OK):
        log.logging.debug("creating file: {}".format(fullpath))
        with open(fullpath, 'w+'):
            pass
        os.chmod(fullpath, stat.S_IWGRP)


def delete_file_if_exists(fullpath):
    """The function checks if a file exists in a directory and deletes it if it exists."""
    if os.path.isfile(fullpath):
        log.logging.debug("deleting file: {}".format(fullpath))
        os.remove(fullpath)


def file_get_contents(filepath):
    """A wrapper function to easily read the contents of a file."""
    with open(filepath) as filehandler:
        return filehandler.read()


def write_into_file(filepath, content):
    """a wrapper around the os functions to easily write text into a file"""
    with open(filepath, 'w') as tmpfile:
        tmpfile.write(content)


# -------------------------
# Data Type Operations

def is_float(value):
    """tries to convert input value into a float value. Returns Boolean"""
    try:
        float(value)
        return True
    except ValueError:
        return False


def is_int(value):
    """tries to convert the input value into a int value"""
    try:
        if is_float(value):
            if float(value) % 1 == 0:
                return True
        return False
    except ValueError:
        return False


def csv_to_array(inputstr: str):
    """Convert a CSV string into a Python compatible array"""
    return csv.reader(inputstr, delimiter=',')


def urlencode(inputstr: str):
    """Convert a String to a URL Encoded String"""
    inputstr = urllib.parse.quote_plus(inputstr)
    return inputstr.replace("+", "%20")


# -------------------------
# Time Operations

def time_duration_diff(date1, date2):
    """The function calculates the difference between two dates and returns them as a string."""
    diff = date2 - date1
    diff = diff.total_seconds()
    if diff < 1:
        diff = 1

    tokens = {
        31536000: "year",
        2592000: "month",
        604800: "week",
        86400: "day",
        3600: "hour",
        60: "minute",
        1: "second"
    }

    for unit, text in sorted(tokens.items(), reverse=True):
        unit = int(unit)
        if diff < unit:
            continue
        number_of_units = math.floor(diff / unit)
        ending = ""
        if number_of_units > 1:
            ending = "s"
        return str(number_of_units) + " " + text + ending

    return ""
