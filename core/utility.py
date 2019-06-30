import os
import log
from datetime import datetime
import math


def create_folder_if_not_exists(filepath):
    if not os.path.exists(filepath):
        log.logging.debug("creating folder: " + filepath)
        os.makedirs(filepath)


def create_file_if_not_exists(fullpath):
    if not os.path.isfile(fullpath) or not os.access(fullpath, os.R_OK):
        log.logging.debug("creating file: " + fullpath)
        with open(fullpath, 'w+'):
            pass


def delete_file_if_exists(fullpath):
    if os.path.isfile(fullpath):
        log.logging.debug("deleting file: " + fullpath)
        os.remove(fullpath)


def file_get_contents(filepath):
    with open(filepath) as f:
        return f.read()


def time_duration_diff(d1, d2):
    diff = d2 - d1
    diff = diff.seconds
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
        numberOfUnits = math.floor(diff / unit)
        ending = ""
        if numberOfUnits > 1:
            ending = "s"
        return str(numberOfUnits) + " " + text + ending

    return ""
