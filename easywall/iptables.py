"""the iptables module is a wrapper around the iptables software"""
from logging import debug, error

from easywall.config import Config
from easywall.utility import (create_folder_if_not_exists,
                              execute_os_command)


class Iptables(object):
    """the class contains functions that interact with the iptables software"""

    def __init__(self):
        """the init function creates some useful class variables"""
        debug("Setting up iptables...")
        self.config = Config("config/easywall.ini")
        self.ipv6 = bool(self.config.get_value("IPV6", "enabled"))
        self.iptables_bin = self.config.get_value("EXEC", "iptables")
        self.iptables_bin_save = self.config.get_value(
            "EXEC", "iptables-save")
        self.iptables_bin_restore = self.config.get_value(
            "EXEC", "iptables-restore")
        if self.ipv6 is True:
            debug("IPV6 is enabled")
            self.ip6tables_bin = self.config.get_value("EXEC", "ip6tables")
            self.ip6tables_bin_save = self.config.get_value(
                "EXEC", "ip6tables-save")
            self.ip6tables_bin_restore = self.config.get_value(
                "EXEC", "ip6tables-restore")

    def add_policy(self, chain, target):
        """the function creates a new policy in iptables"""
        debug("adding policy for chain " +
              chain + " and target " + target)
        if target == "ACCEPT" or target == "DROP":
            execute_os_command(
                self.iptables_bin + " -P " + chain + " " + target)
            if self.ipv6 is True:
                execute_os_command(
                    self.ip6tables_bin + " -P " + chain + " " + target)
        else:
            error("Invalid Target for addPolicy " + target)

    def add_chain(self, chain):
        """the function creates a new chain in iptables"""
        debug("adding chain " + chain)
        execute_os_command(self.iptables_bin + " -N " + chain)
        if self.ipv6 is True:
            execute_os_command(self.ip6tables_bin + " -N " + chain)

    def add_append(self, chain, rule, onlyv6=False, onlyv4=False):
        """the function creates a new append in iptables"""
        if onlyv4 is True or (onlyv6 is False and onlyv4 is False):
            debug(
                "adding append for ipv4, chain: " + chain + ", rule: " + rule)
            execute_os_command(self.iptables_bin + " -A " + chain + " " + rule)
        if self.ipv6 is True and(
                onlyv6 is True or(onlyv6 is False and onlyv4 is False)):
            debug(
                "adding append for ipv6, chain: " + chain + ", rule: " + rule)
            execute_os_command(
                self.ip6tables_bin + " -A " + chain + " " + rule)

    def flush(self, chain=""):
        """the function flushes a iptables chain or all chains"""
        debug("flushing iptables chain: " + chain)
        execute_os_command(self.iptables_bin + " -F " + chain)
        if self.ipv6 is True:
            execute_os_command(self.ip6tables_bin + " -F " + chain)

    def delete_chain(self, chain=""):
        """the function deletes a chain in iptables"""
        debug("deleting chain " + chain)
        execute_os_command(self.iptables_bin + " -X " + chain)
        if self.ipv6 is True:
            execute_os_command(self.ip6tables_bin + " -X " + chain)

    def reset(self):
        """the function resets iptables to a clean state"""
        debug("resetting iptables to empty configuration")
        self.add_policy("INPUT", "ACCEPT")
        self.add_policy("OUTPUT", "ACCEPT")
        self.add_policy("FORWARD", "ACCEPT")
        self.flush()
        self.delete_chain()

    def list(self):
        """the function lists all iptables rules"""
        execute_os_command(self.iptables_bin + " -L")

    def save(self):
        """the function saves the current iptables state into a file"""
        debug("Starting Firewall Rule Backup...")
        # Create Backup Directory if not exists
        filepath = self.config.get_value("BACKUP", "filepath")
        create_folder_if_not_exists(filepath)

        # backing up ipv4 iptables rules
        debug("Backing up ipv4 rules...")
        filename = self.config.get_value("BACKUP", "ipv4filename")
        open(filepath + "/" + filename, 'w')
        self.save_execute(self.iptables_bin_save, filepath, filename)

        # backing up ipv6 iptables rules
        if self.ipv6 is True:
            debug("Backing up ipv6 rules...")
            filename = self.config.get_value("BACKUP", "ipv6filename")
            open(filepath + "/" + filename, 'w')
            self.save_execute(self.ip6tables_bin_save, filepath, filename)

    def save_execute(self, binary, filepath, filename):
        """the function executes the save of iptables into a file"""
        execute_os_command(
            binary + " | while read IN ; do echo $IN >> " + filepath + "/" +
            filename + " ; done")

    def restore(self):
        """the function restores iptables rules from a file"""
        debug("Starting Firewall Rule Restore...")
        filepath = self.config.get_value("BACKUP", "filepath")
        create_folder_if_not_exists(filepath)

        debug("Restoring ipv4 rules...")
        filename = self.config.get_value("BACKUP", "ipv4filename")
        execute_os_command(self.iptables_bin_restore + " < " +
                           filepath + "/" + filename)

        if self.ipv6 is True:
            debug("Restoring ipv6 rules...")
            filename = self.config.get_value("BACKUP", "ipv6filename")
            execute_os_command(self.ip6tables_bin_restore + " < " +
                               filepath + "/" + filename)
