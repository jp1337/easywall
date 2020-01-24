"""the module contains the core functions of easywall"""
from logging import info
from time import sleep

from watchdog.events import FileSystemEventHandler
from watchdog.observers import Observer

from easywall.easywall import Easywall
from easywall.config import Config
from easywall.log import Log
from easywall.utility import (create_file_if_not_exists,
                                  create_folder_if_not_exists,
                                  delete_file_if_exists, file_exists)


class ModifiedHandler(FileSystemEventHandler):
    """the class contains a event handler which listens on specified file types"""

    def on_any_event(self, event):
        """
        the function overrides the empty event handler for every file change in the given directory
        """
        if event.src_path.endswith(".txt"):
            info(
                "file modification occured. filename: " + event.src_path)
            while file_exists(".running"):
                sleep(2)
            Easywall()


def run():
    """this is the main function of the program"""
    # Startup Process
    config = Config("config/easywall.ini")
    loglevel = config.get_value("LOG", "level")
    to_stdout = config.get_value("LOG", "to_stdout")
    to_files = config.get_value("LOG", "to_files")
    logpath = config.get_value("LOG", "filepath")
    logfile = config.get_value("LOG", "filename")
    log = Log(loglevel, to_stdout, to_files, logpath, logfile)
    info("Starting up easywall...")

    ensure_rules_files(config)
    event_handler = ModifiedHandler()
    observer = Observer()
    observer.schedule(
        event_handler, config.get_value("RULES", "filepath"))
    observer.start()
    info("easywall is up and running.")

    # waiting for file modifications
    try:
        while True:
            sleep(2)
    except KeyboardInterrupt:
        info("KeyboardInterrupt received, starting shutdown")
    finally:
        shutdown(observer, config, log)


def shutdown(observer, config, log):
    """this function executes a shutdown of easywall"""
    info("Shutting down easywall...")
    observer.stop()
    delete_file_if_exists(".running")
    delete_file_if_exists(
        config.get_value("ACCEPTANCE", "filename"))
    observer.join()
    log.close_logging()
    info("easywall was stopped gracefully")
    exit(0)


def ensure_rules_files(cfg):
    """the function creates several files if they don't exist"""
    for ruletype in ["blacklist", "whitelist", "tcp", "udp", "custom"]:
        filepath = cfg.get_value("RULES", "filepath")
        filename = cfg.get_value("RULES", ruletype)
        create_folder_if_not_exists(filepath)
        create_file_if_not_exists(filepath + "/" + filename)


if __name__ == "__main__":
    run()
