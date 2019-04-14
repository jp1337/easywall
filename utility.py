import os


def createFolderIfNotExists(filepath):
    if not os.path.exists(filepath):
        os.makedirs(filepath)


def createFileIfNotExists(fullpath):
    if not os.path.isfile(fullpath) or not os.access(fullpath, os.R_OK):
        with open(fullpath, 'w+'):
            pass


def deleteFileIfExists(fullpath):
    if os.path.isfile(fullpath):
        os.remove(fullpath)
