"""TODO: Doku."""
from enum import Enum
from logging import debug, info

from easywall.config import Config
from easywall.utility import (create_file_if_not_exists,
                              create_folder_if_not_exists,
                              delete_file_if_exists, execute_os_command,
                              file_get_contents)


class Target(Enum):
    """TODO: Doku."""

    ACCEPT = "ACCEPT"
    DROP = "DROP"


class Chain(Enum):
    """TODO: Doku."""

    INPUT = "INPUT"
    FORWARD = "FORWARD"
    OUTPUT = "OUTPUT"
    PREROUTING = "PREROUTING"
    SSHBRUTE = "SSHBRUTE"
    INVALIDDROP = "INVALIDDROP"
    PORTSCAN = "PORTSCAN"
    ICMPFLOOD = "ICMPFLOOD"


class Iptables():
    """TODO: Doku."""

    def __init__(self, cfg: Config) -> None:
        """TODO: Doku."""
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
        """Create a new policy in iptables firewall by using the os command."""
        option = "-P"

        execute_os_command("{} {} {} {}".format(
            self.iptables_bin, option, chain.value, target.value))
        if self.ipv6 is True:
            execute_os_command("{} {} {} {}".format(
                self.ip6tables_bin, option, chain.value, target.value))

        info("iptables policy added for chain {} and target {}".format(chain.value, target.value))

    def add_chain(self, chain: str) -> None:
        """Create a new custom chain in iptables."""
        option = "-N"

        execute_os_command("{} {} {}".format(self.iptables_bin, option, chain))
        if self.ipv6 is True:
            execute_os_command("{} {} {}".format(self.ip6tables_bin, option, chain))

        info("iptables chain {} added".format(chain))

    def add_append(self, chain: Chain, rule: str,
                   onlyv6: bool = False, onlyv4: bool = False, table: str = "") -> None:
        """Create a new append in iptables."""
        option = "-A"

        if table != "":
            if not table.startswith("-t"):
                table = "-t " + table

        if onlyv4 is True or (onlyv6 is False and onlyv4 is False):
            execute_os_command("{} {} {} {} {}".format(
                self.iptables_bin, table, option, chain.value, rule))
            info("append for ipv4: table: {}, chain: {}, rule: {} added".format(
                table, chain.value, rule))

        if self.ipv6 is True and (onlyv6 is True or (onlyv6 is False and onlyv4 is False)):
            execute_os_command("{} {} {} {} {}".format(
                self.ip6tables_bin, table, option, chain.value, rule))
            info("append for ipv6: table: {}, chain: {}, rule: {} added".format(
                table, chain.value, rule))

    def insert(self, chain: Chain, rule: str,
               onlyv6: bool = False, onlyv4: bool = False, table: str = "") -> None:
        """TODO: Doku."""
        option = "-I"

        if table != "":
            if not table.startswith("-t"):
                table = "-t " + table

        if onlyv4 is True or (onlyv6 is False and onlyv4 is False):
            execute_os_command("{} {} {} {} {}".format(
                self.iptables_bin, table, option, chain.value, rule))
            info("insert for ipv4, table: {}, chain: {}, rule: {} added".format(
                table, chain.value, rule))

        if self.ipv6 is True and (onlyv6 is True or (onlyv6 is False and onlyv4 is False)):
            execute_os_command("{} {} {} {} {}".format(
                self.ip6tables_bin, table, option, chain.value, rule))
            info("insert for ipv6, table: {}, chain: {}, rule: {} added".format(
                table, chain.value, rule))

    def add_custom(self, rule: str) -> None:
        """TODO: Doku."""
        execute_os_command("{} {}".format(self.iptables_bin, rule))
        if self.ipv6 is True:
            execute_os_command("{} {}".format(self.ip6tables_bin, rule))

        info("iptables custom rule added: {}".format(rule))

    def flush(self, chain: str = "", table: str = "") -> None:
        """Flush chain or all chains in iptables firewall."""
        option = "-F"

        if table != "":
            if not table.startswith("-t"):
                table = "-t " + table

        execute_os_command("{} {} {} {}".format(self.iptables_bin, table, option, chain))
        if self.ipv6 is True:
            execute_os_command("{} {} {} {}".format(self.ip6tables_bin, table, option, chain))

        if chain != "":
            info("iptables chain {} flushed".format(chain))
        else:
            info("all iptables chains flushed")

    def delete_chain(self, chain: str = "") -> None:
        """Delete a chain or all chains in iptables firewall."""
        execute_os_command("{} -X {}".format(self.iptables_bin, chain))
        if self.ipv6 is True:
            execute_os_command("{} -X {}".format(self.ip6tables_bin, chain))

        if chain != "":
            info("iptables chain {} deleted".format(chain))
        else:
            info("all iptables chains deleted")

    def reset(self) -> None:
        """Reset iptables and allows all connections to the system and from the system."""
        self.add_policy(Chain.INPUT, Target.ACCEPT)
        self.add_policy(Chain.OUTPUT, Target.ACCEPT)
        self.add_policy(Chain.FORWARD, Target.ACCEPT)
        self.flush(table="raw")
        self.flush(table="nat")
        self.flush(table="mangle")
        self.flush()
        self.delete_chain()

        info("incoming and outgoing connections have been opened and chains have been deleted")

    def status(self) -> str:
        """List the iptables configuration as string.

        [WARNING] this is not machine readable!
        """
        tmpfile = ".iptables_list"
        execute_os_command("{} -L > {}".format(self.iptables_bin, tmpfile))
        content = file_get_contents(tmpfile)
        delete_file_if_exists(tmpfile)
        return content

    def save(self) -> None:
        """Save the current iptables state into a file."""
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
        """Restore a backup of a previously saved backup."""
        create_folder_if_not_exists(self.backup_path)

        execute_os_command("{} < {}/{}".format(self.iptables_bin_restore,
                                               self.backup_path, self.backup_file_ipv4))
        debug("ipv4 rules restored")

        if self.ipv6 is True:
            execute_os_command("{} < {}/{}".format(self.ip6tables_bin_restore,
                                                   self.backup_path, self.backup_file_ipv6))
            debug("ipv6 rules restored")

        info("restores iptables state from previous created backup")
