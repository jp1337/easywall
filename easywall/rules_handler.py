"""TODO: Doku."""
from logging import error

from yaml import Dumper, Loader, dump, load

from easywall.utility import (create_file_if_not_exists, file_exists,
                              file_get_contents, format_exception,
                              write_into_file)


class RulesHandler():
    """TODO: Doku."""

    def __init__(self) -> None:
        """TODO: Doku."""
        self.types = ["blacklist", "whitelist", "tcp", "udp", "custom", "forwarding"]
        self.states = ["current", "new", "backup"]
        self.filename = "rules.yml"
        self.filepath = "{}/{}".format("config", self.filename)
        self.ensure_file_exists()
        self.rules = self.load()

    def load(self) -> dict:
        """TODO: Doku."""
        content = file_get_contents(self.filepath)
        return dict(load(content, Loader=Loader))

    def save(self) -> bool:
        """TODO: Doku."""
        try:
            data = dump(data=self.rules, Dumper=Dumper, default_flow_style=False)
            write_into_file(self.filepath, data)
            return True
        except Exception as exc:
            error(format_exception(exc))
            return False

    def ensure_file_exists(self) -> None:
        """TODO: Doku."""
        if not file_exists(self.filepath):
            create_file_if_not_exists(self.filepath)

            template: dict = {}
            for state in self.states:
                template[state] = {}
                for ruletype in self.types:
                    template[state][ruletype] = []

            self.rules = template
            self.save()

    def get_current_rules(self, ruletype: str) -> list:
        """TODO: Doku."""
        return list(self.rules["current"][ruletype])

    def get_new_rules(self, ruletype: str) -> list:
        """TODO: Doku."""
        return list(self.rules["new"][ruletype])

    def get_backup_rules(self, ruletype: str) -> list:
        """TODO: Doku."""
        return list(self.rules["backup"][ruletype])

    def get_rules_for_web(self, ruletype: str) -> list:
        """TODO: Doku."""
        if self.diff_new_current(ruletype):
            return self.get_new_rules(ruletype)
        return self.get_current_rules(ruletype)

    def backup_current_rules(self) -> None:
        """TODO: Doku."""
        self.rules["backup"] = self.rules["current"].copy()
        self.save()

    def apply_new_rules(self) -> None:
        """TODO: Doku."""
        self.rules["current"] = self.rules["new"].copy()
        self.save()

    def rollback_from_backup(self) -> None:
        """TODO: Doku."""
        self.rules["current"] = self.rules["backup"].copy()
        self.save()

    def diff_new_current(self, ruletype: str) -> bool:
        """
        TODO: Doku.

        True = There are differences between new and current
        False = There are no differences between new and current
        """
        state = False

        new = self.get_new_rules(ruletype)
        current = self.get_current_rules(ruletype)
        if new != current:
            return True

        return state

    def save_new_rules(self, ruletype: str, rules: list) -> None:
        """TODO: Doku."""
        rules = list(filter(None, rules))
        self.rules["new"][ruletype] = rules
        self.save()
