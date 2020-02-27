"""the module contains the core functions of easywall"""
from logging import info, error
from time import sleep

from watchdog.events import FileSystemEventHandler
from watchdog.observers import Observer

from easywall.config import Config
from easywall.easywall import Easywall
from easywall.log import Log
from easywall.utility import (create_file_if_not_exists,
                              create_folder_if_not_exists,
                              delete_file_if_exists, file_exists)

CONFIG_PATH = "config/easywall.ini"


class ModifiedHandler(FileSystemEventHandler):
    """the class contains a event handler which listens on specified file types"""

    def on_any_event(self, event):
        """
        the function overrides the empty event handler for every file change in the given directory
        """
        if event.src_path.endswith(".txt"):
            info("file modification occured. filename: " + event.src_path)
            while file_exists(".running"):
                sleep(2)
            Easywall(CONFIG_PATH)


def run():
    """
    this is the first and main function of the program
    """
    config = Config(CONFIG_PATH)
    loglevel = config.get_value("LOG", "level")
    to_stdout = config.get_value("LOG", "to_stdout")
    to_files = config.get_value("LOG", "to_files")
    logpath = config.get_value("LOG", "filepath")
    logfile = config.get_value("LOG", "filename")
    log = Log(loglevel, to_stdout, to_files, logpath, logfile)
    info("executing startup sequence...")

    create_rule_files(config)
    event_handler = ModifiedHandler()
    observer = Observer()
    observer.schedule(event_handler, config.get_value("RULES", "filepath"))
    observer.start()
    info("startup sequence successfully finished")
    start_observer(observer, config, log)


def start_observer(observer: Observer, config: Config, log: Log):
    """
    this function is called to keep the main process running
    if someone is pressing ctrl + C the software will initiate the stop process
    """
    try:
        while True:
            sleep(2)
    except KeyboardInterrupt:
        info("KeyboardInterrupt received, starting shutdown")
    except Exception as exc:
        error("Got error message: {}".format(exc))
    finally:
        shutdown(observer, config, log)


def shutdown(observer: Observer, config: Config, log: Log):
    """
    the function stops all threads and shuts the software down gracefully
    """
    info("starting shutdown...")
    observer.stop()
    delete_file_if_exists(".running")
    delete_file_if_exists(config.get_value("ACCEPTANCE", "filename"))
    observer.join()
    info("shutdown successfully completed")
    log.close_logging()
    exit(0)


def create_rule_files(cfg: Config):
    """
    the function checks if the rule files exist and creates them if they don't exist
    """
    filepath = cfg.get_value("RULES", "filepath")
    create_folder_if_not_exists(filepath)
    filename = ""

    for ruletype in ["blacklist", "whitelist", "tcp", "udp", "custom"]:
        filename = cfg.get_value("RULES", ruletype)
        create_file_if_not_exists("{}/{}".format(filepath, filename))


if __name__ == "__main__":
    run()
