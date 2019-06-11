import os
import log


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
