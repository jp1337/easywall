"""TODO: Doku."""
from logging import info
from time import sleep
from typing import Callable

from watchdog.events import FileSystemEvent, FileSystemEventHandler
from watchdog.observers import Observer

from easywall.config import Config
from easywall.easywall import Easywall
from easywall.log import Log
from easywall.rules_handler import RulesHandler
from easywall.utility import delete_file_if_exists

CONFIG_PATH = "config/easywall.ini"
LOG_CONFIG_PATH = "config/log.ini"


class ModifiedHandler(FileSystemEventHandler):
    """TODO: Doku."""

    def __init__(self, apply: Callable) -> None:
        """TODO: Doku."""
        self.apply = apply

    def on_created(self, event: FileSystemEvent) -> None:
        """TODO: Doku."""
        if event.src_path.endswith("apply"):
            info("file was created. filename: {}".format(event.src_path))
            self.apply(event.src_path)


class Main():
    """TODO: Doku."""

    def __init__(self) -> None:
        """TODO: Doku."""
        self.cfg = Config(CONFIG_PATH)
        self.cfg_log = Config(LOG_CONFIG_PATH)

        loglevel = self.cfg_log.get_value("LOG", "level")
        to_stdout = self.cfg_log.get_value("LOG", "to_stdout")
        to_files = self.cfg_log.get_value("LOG", "to_files")
        logpath = self.cfg_log.get_value("LOG", "filepath")
        logfile = self.cfg_log.get_value("LOG", "filename")
        self.log = Log(str(loglevel), bool(to_stdout), bool(to_files), str(logpath), str(logfile))

        info("starting easywall")

        self.rules_handler = RulesHandler()
        self.rules_handler.ensure_files_exist()
        self.easywall = Easywall(self.cfg)
        self.event_handler = ModifiedHandler(self.apply)
        self.observer = Observer()
        self.stop_flag = False

        info("easywall has been started")

    def apply(self, filename: str) -> None:
        """TODO: Doku."""
        info("starting apply process from easywall")
        delete_file_if_exists(filename)
        self.easywall.apply()

    def start_observer(self) -> None:
        """
        Keep the main process running until it should be stopped.

        if someone is pressing ctrl + C the software will initiate the stop process
        """
        self.observer.schedule(self.event_handler, ".")
        self.observer.start()

        try:
            while not self.stop_flag:
                sleep(2)
        except KeyboardInterrupt:
            info("KeyboardInterrupt received, starting shutdown")
        finally:
            self.shutdown()

    def shutdown(self) -> None:
        """Stop all threads and shut the software down gracefully."""
        info("starting shutdown")

        self.observer.stop()
        delete_file_if_exists(".acceptance")
        self.observer.join()

        info("shutdown completed")
        self.log.close_logging()


if __name__ == "__main__":
    MAIN = Main()
    MAIN.start_observer()
