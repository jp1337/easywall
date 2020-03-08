"""
TODO: Doku
"""
from easywall.utility import (create_file_if_not_exists,
                              create_folder_if_not_exists, file_get_contents,
                              write_into_file)


class RulesHandler(object):
    """
    TODO: Doku
    """

    def __init__(self) -> None:
        self.rulesfolder = "rules"
        self.types = ["blacklist", "whitelist", "tcp", "udp", "custom"]
        self.states = ["current", "new", "backup"]

    def get_current_rules(self, ruletype: str) -> list:
        """
        TODO: Doku
        """
        return file_get_contents("{}/current/{}".format(self.rulesfolder, ruletype)).splitlines()

    def backup_current_rules(self) -> None:
        """
        TODO: Doku
        """
        self.copy_rules("current", "backup")

    def apply_new_rules(self) -> None:
        """
        TODO: Doku
        """
        self.copy_rules("new", "current")

    def rollback_from_backup(self) -> None:
        """
        TODO: Doku
        """
        self.copy_rules("backup", "current")

    def copy_rules(self, source: str, dest: str) -> None:
        """
        TODO: Doku
        """
        for ruletype in self.types:
            content = file_get_contents("{}/{}/{}".format(self.rulesfolder, source, ruletype))
            write_into_file("{}/{}/{}".format(self.rulesfolder, dest, ruletype), content)

    def rules_firstrun(self) -> None:
        """
        TODO: Doku
        """
        create_folder_if_not_exists(self.rulesfolder)

        for state in self.states:
            create_folder_if_not_exists("{}/{}".format(self.rulesfolder, state))

            for ruletype in self.types:
                create_file_if_not_exists("{}/{}/{}".format(self.rulesfolder, state, ruletype))
