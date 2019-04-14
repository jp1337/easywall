import os
import log


def createFolderIfNotExists(filepath):
    if not os.path.exists(filepath):
        log.logging.debug("creating folder: " + filepath)
        os.makedirs(filepath)


def createFileIfNotExists(fullpath):
    if not os.path.isfile(fullpath) or not os.access(fullpath, os.R_OK):
        log.logging.debug("creating file: " + fullpath)
        with open(fullpath, 'w+'):
            pass


def deleteFileIfExists(fullpath):
    if os.path.isfile(fullpath):
        log.logging.debug("deleting file: " + fullpath)
        os.remove(fullpath)
