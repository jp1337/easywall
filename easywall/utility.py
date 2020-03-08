"""
this module exports useful functions which are not implemented in python by default
"""
from csv import reader
from datetime import datetime
from io import StringIO
from math import floor
from os import R_OK, access, chmod, makedirs, path, remove, rename, system
from traceback import TracebackException
from urllib import parse

# -------------------------
# File Operations


def create_folder_if_not_exists(filepath: str):
    """Checks if a folder exists and creates if it does not exist"""
    if not path.exists(filepath):
        makedirs(filepath)


def create_file_if_not_exists(fullpath: str):
    """The function creates a file if it does not already exist."""
    if not path.isfile(fullpath) or not access(fullpath, R_OK):
        with open(fullpath, 'w+'):
            pass
        chmod(fullpath, 0o770)


def delete_file_if_exists(fullpath: str):
    """The function checks if a file exists in a directory and deletes it if it exists."""
    if path.isfile(fullpath):
        remove(fullpath)


def file_get_contents(filepath: str) -> str:
    """A wrapper function to easily read the contents of a file."""
    with open(filepath) as filehandler:
        return filehandler.read()


def write_into_file(filepath: str, content: str):
    """a wrapper around the os functions to easily write text into a file"""
    with open(filepath, 'w') as tmpfile:
        tmpfile.write(content)


def get_abs_path_of_filepath(filepath: str):
    """returns the absolute path of a path containing a filename"""
    return path.dirname(path.realpath(filepath))


def rename_file(oldpath: str, newpath: str):
    """renames a the file from the absolute path oldpath into the file from newpath"""
    rename(oldpath, newpath)


def file_exists(filepath: str) -> bool:
    """
    the function checks if a fiven file exists on the system.

    [Data Types] boolean
    """
    return path.isfile(filepath)


def folder_exists(folder_path: str) -> bool:
    """
    the function checks if a given folder exists on the system.

    [Data Types] bool
    """
    return path.isdir(folder_path)

# -------------------------
# Data Type Operations


def is_float(value) -> bool:
    """tries to convert input value into a float value. Returns Boolean"""
    try:
        float(value)
        return True
    except ValueError:
        return False


def is_int(value) -> bool:
    """tries to convert the input value into a int value"""
    if is_float(value):
        if float(value) % 1 == 0:
            return True
    return False


def csv_to_array(inputstr: str, delimiter: str) -> list:
    """Convert a CSV string into a Python compatible array"""
    results = []
    strio = StringIO(inputstr)
    csv_reader = reader(strio, delimiter=delimiter)
    for row in csv_reader:
        for element in row:
            results.append(element)
    return results


def urlencode(inputstr: str) -> str:
    """Convert a String to a URL Encoded String"""
    inputstr = parse.quote_plus(inputstr)
    return inputstr.replace("+", "%20")


def format_exception(exc: Exception) -> str:
    """Converts a exception object to a readable string"""
    return "".join(TracebackException.from_exception(exc).format())

# -------------------------
# Time Operations


def time_duration_diff(date1: datetime, date2: datetime):
    """The function calculates the difference between two dates and returns them as a string."""
    result = ""
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
        number_of_units = floor(diff / unit)
        ending = ""
        if number_of_units > 1:
            ending = "s"
        result = str(number_of_units) + " " + text + ending

    return result

# -------------------------
# System Operations


def execute_os_command(command: str) -> bool:
    """this function executes a command on the operating system"""
    if system(command) > 0:
        return False
    return True
