"""TODO: Doku."""
from typing import List

from easywall.utility import (create_file_if_not_exists,
                              create_folder_if_not_exists, file_get_contents,
                              write_into_file)


class RulesHandler():
    """TODO: Doku."""

    def __init__(self) -> None:
        self.rulesfolder = "rules"
        self.types = ["blacklist", "whitelist", "tcp", "udp", "custom", "forwarding"]
        self.states = ["current", "new", "backup"]

    def get_current_rules(self, ruletype: str) -> List[str]:
        """TODO: Doku."""
        return file_get_contents("{}/current/{}".format(self.rulesfolder, ruletype)).splitlines()

    def get_new_rules(self, ruletype: str) -> List[str]:
        """TODO: Doku."""
        return file_get_contents("{}/new/{}".format(self.rulesfolder, ruletype)).splitlines()

    def get_rules_for_web(self, ruletype: str) -> List[str]:
        """TODO: Doku."""
        if self.diff_new_current(ruletype):
            return self.get_new_rules(ruletype)
        return self.get_current_rules(ruletype)

    def backup_current_rules(self) -> None:
        """TODO: Doku."""
        self.copy_rules("current", "backup")

    def apply_new_rules(self) -> None:
        """TODO: Doku."""
        self.copy_rules("new", "current")

    def rollback_from_backup(self) -> None:
        """TODO: Doku."""
        self.copy_rules("backup", "current")

    def copy_rules(self, source: str, dest: str) -> None:
        """TODO: Doku."""
        for ruletype in self.types:
            content = file_get_contents("{}/{}/{}".format(self.rulesfolder, source, ruletype))
            write_into_file("{}/{}/{}".format(self.rulesfolder, dest, ruletype), content)

    def ensure_files_exist(self) -> None:
        """TODO: Doku."""
        create_folder_if_not_exists(self.rulesfolder)

        for state in self.states:
            create_folder_if_not_exists("{}/{}".format(self.rulesfolder, state))

            for ruletype in self.types:
                create_file_if_not_exists("{}/{}/{}".format(self.rulesfolder, state, ruletype))

    def diff_new_current(self, ruletype: str) -> bool:
        """
        TODO: Doku

        True = There are differences between new and current
        False = There are no differences between new and current
        """
        state = False

        new = file_get_contents("{}/new/{}".format(self.rulesfolder, ruletype))
        current = file_get_contents("{}/current/{}".format(self.rulesfolder, ruletype))
        if new != current:
            return True

        return state

    def save_new_rules(self, ruletype: str, rules: list) -> None:
        """TODO: Doku."""
        rules = list(filter(None, rules))
        write_into_file("{}/new/{}".format(self.rulesfolder, ruletype), '\n'.join(rules))
