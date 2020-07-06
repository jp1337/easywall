"""
TODO: Doku
"""
from enum import Enum
from logging import debug, info

from easywall.config import Config
from easywall.utility import (create_file_if_not_exists,
                              create_folder_if_not_exists,
                              delete_file_if_exists, execute_os_command,
                              file_get_contents)


class Target(Enum):
    """
    TODO: Doku
    """
    ACCEPT = "ACCEPT"
    DROP = "DROP"


class Chain(Enum):
    """
    TODO: Doku
    """
    INPUT = "INPUT"
    FORWARD = "FORWARD"
    OUTPUT = "OUTPUT"


class Iptables(object):
    """
    TODO: Doku
    """

    def __init__(self, cfg: Config) -> None:
        self.cfg = cfg

        self.iptables_bin = self.cfg.get_value("EXEC", "iptables")
        self.iptables_bin_save = self.cfg.get_value("EXEC", "iptables-save")
        self.iptables_bin_restore = self.cfg.get_value("EXEC", "iptables-restore")

        self.ip6tables_bin = self.cfg.get_value("EXEC", "ip6tables")
        self.ip6tables_bin_save = self.cfg.get_value("EXEC", "ip6tables-save")
        self.ip6tables_bin_restore = self.cfg.get_value("EXEC", "ip6tables-restore")

        self.ipv6 = self.cfg.get_value("IPV6", "enabled")
        if self.ipv6 is True:
            debug("IPV6 is enabled")

        self.backup_path = "backup"
        self.backup_file_ipv4 = "iptables_v4_backup"
        self.backup_file_ipv6 = "iptables_v6_backup"

    def add_policy(self, chain: Chain, target: Target) -> None:
        """
        the function creates a new policy in iptables firewall by using the os command
        """
        execute_os_command("{} -P {} {}".format(self.iptables_bin, chain, target))
        if self.ipv6 is True:
            execute_os_command("{} -P {} {}".format(self.ip6tables_bin, chain, target))

        info("iptables policy added for chain {} and target {}".format(chain, target))

    def add_chain(self, chain: str) -> None:
        """
        the function creates a new custom chain in iptables
        """
        execute_os_command("{} -N {}".format(self.iptables_bin, chain))
        if self.ipv6 is True:
            execute_os_command("{} -N {}".format(self.ip6tables_bin, chain))

        info("iptables chain {} added".format(chain))

    def add_append(self, chain: str, rule: str, onlyv6=False, onlyv4=False) -> None:
        """
        the function creates a new append in iptables
        """
        if onlyv4 is True or (onlyv6 is False and onlyv4 is False):
            execute_os_command("{} -A {} {}".format(self.iptables_bin, chain, rule))
            info("append for ipv4, chain: {}, rule: {} added".format(chain, rule))

        if self.ipv6 is True and (onlyv6 is True or (onlyv6 is False and onlyv4 is False)):
            execute_os_command("{} -A {} {}".format(self.ip6tables_bin, chain, rule))
            info("append for ipv6, chain: {}, rule: {} added".format(chain, rule))

    def flush(self, chain: str = "") -> None:
        """
        the function flushes chain or all chains in iptables firewall
        """
        execute_os_command("{} -F {}".format(self.iptables_bin, chain))
        if self.ipv6 is True:
            execute_os_command("{} -F {}".format(self.ip6tables_bin, chain))

        if chain != "":
            info("iptables chain {} flushed".format(chain))
        else:
            info("all iptables chains flushed")

    def delete_chain(self, chain: str = ""):
        """
        the function deletes a chain or all chains in iptables firewall
        """
        execute_os_command("{} -X {}".format(self.iptables_bin, chain))
        if self.ipv6 is True:
            execute_os_command("{} -X {}".format(self.ip6tables_bin, chain))

        if chain != "":
            info("iptables chain {} deleted".format(chain))
        else:
            info("all iptables chains deleted")

    def reset(self):
        """
        the function resets iptables and allows all connections to the system and from the system
        """
        self.add_policy("INPUT", "ACCEPT")
        self.add_policy("OUTPUT", "ACCEPT")
        self.add_policy("FORWARD", "ACCEPT")
        self.flush(table="raw")
        self.flush(table="nat")
        self.flush(table="mangle")
        self.flush()
        self.delete_chain()

        info("incoming and outgoing connections have been opened and chains have been deleted")

    def status(self) -> str:
        """
        the function lists the iptables configuration as string
        this is not machine readable!
        """
        tmpfile = ".iptables_list"
        execute_os_command("{} -L > {}".format(self.iptables_bin, tmpfile))
        content = file_get_contents(tmpfile)
        delete_file_if_exists(tmpfile)
        return content

    def save(self) -> None:
        """
        the function saves the current iptables state into a file
        """
        create_folder_if_not_exists(self.backup_path)

        create_file_if_not_exists("{}/{}".format(self.backup_path, self.backup_file_ipv4))
        execute_os_command("{} >> {}/{}".format(self.iptables_bin_save,
                                                self.backup_path, self.backup_file_ipv4))
        debug("backup for ipv4 rules created")

        if self.ipv6 is True:
            create_file_if_not_exists("{}/{}".format(self.backup_path, self.backup_file_ipv6))
            execute_os_command("{} >> {}/{}".format(self.ip6tables_bin_save,
                                                    self.backup_path, self.backup_file_ipv6))
            debug("backup of ipv6 rules created")

        info("backup of iptables configuration created")

    def restore(self) -> None:
        """
        the function restores a backup of a previously saved backup
        """
        create_folder_if_not_exists(self.backup_path)

        execute_os_command("{} < {}/{}".format(self.iptables_bin_restore,
                                               self.backup_path, self.backup_file_ipv4))
        debug("ipv4 rules restored")

        if self.ipv6 is True:
            execute_os_command("{} < {}/{}".format(self.ip6tables_bin_restore,
                                                   self.backup_path, self.backup_file_ipv6))
            debug("ipv6 rules restored")

        info("restores iptables state from previous created backup")
