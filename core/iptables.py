import config
import os
import log
import utility


class iptables(object):
    def __init__(self):
        log.logging.debug("Setting up iptables...")
        self.config = config.config("config/config.ini")
        self.ipv6 = bool(self.config.get_value("IPV6", "enabled"))
        self.iptables_bin = self.config.get_value("EXEC", "iptables")
        self.iptables_bin_save = self.config.get_value(
            "EXEC", "iptables-save")
        self.iptables_bin_restore = self.config.get_value(
            "EXEC", "iptables-restore")
        if self.ipv6 is True:
            log.logging.debug("IPV6 is enabled")
            self.ip6tables_bin = self.config.get_value("EXEC", "ip6tables")
            self.ip6tables_bin_save = self.config.get_value(
                "EXEC", "ip6tables-save")
            self.ip6tables_bin_restore = self.config.get_value(
                "EXEC", "ip6tables-restore")

    def addPolicy(self, chain, target):
        log.logging.debug("adding policy for chain " +
                          chain + " and target " + target)
        if target == "ACCEPT" or target == "DROP":
            self.system_call(
                self.iptables_bin + " -P " + chain + " " + target)
            if self.ipv6 is True:
                self.system_call(
                    self.ip6tables_bin + " -P " + chain + " " + target)
        else:
            log.logging.error("Invalid Target for addPolicy " + target)

    def addChain(self, chain):
        log.logging.debug("adding chain " + chain)
        self.system_call(self.iptables_bin + " -N " + chain)
        if self.ipv6 is True:
            self.system_call(self.ip6tables_bin + " -N " + chain)

    def addAppend(self, chain, rule, onlyv6=False, onlyv4=False):
        if onlyv4 is True or (onlyv6 is False and onlyv4 is False):
            log.logging.debug(
                "adding append for ipv4, chain: " + chain + ", rule: " + rule)
            self.system_call(self.iptables_bin + " -A " + chain + " " + rule)
        if self.ipv6 is True and(
                onlyv6 is True or(onlyv6 is False and onlyv4 is False)):
            log.logging.debug(
                "adding append for ipv6, chain: " + chain + ", rule: " + rule)
            self.system_call(self.ip6tables_bin + " -A " + chain + " " + rule)

    def flush(self, chain=""):
        log.logging.debug("flushing iptables chain: " + chain)
        self.system_call(self.iptables_bin + " -F " + chain)
        if self.ipv6 is True:
            self.system_call(self.ip6tables_bin + " -F " + chain)

    def deleteChain(self, chain=""):
        log.logging.debug("deleting chain " + chain)
        self.system_call(self.iptables_bin + " -X " + chain)
        if self.ipv6 is True:
            self.system_call(self.ip6tables_bin + " -X " + chain)

    def reset(self):
        log.logging.debug("resetting iptables to empty configuration")
        self.addPolicy("INPUT", "ACCEPT")
        self.addPolicy("OUTPUT", "ACCEPT")
        self.addPolicy("FORWARD", "ACCEPT")
        self.flush()
        self.deleteChain()

    def list(self):
        self.system_call(self.iptables_bin + " -L")

    def save(self):
        log.logging.debug("Starting Firewall Rule Backup...")
        # Create Backup Directory if not exists
        filepath = self.config.get_value("BACKUP", "filepath")
        utility.create_folder_if_not_exists(filepath)

        # backing up ipv4 iptables rules
        log.logging.debug("Backing up ipv4 rules...")
        filename = self.config.get_value("BACKUP", "ipv4filename")
        open(filepath + "/" + filename, 'w')
        self.save_execute(self.iptables_bin_save, filepath, filename)

        # backing up ipv6 iptables rules
        if self.ipv6 is True:
            log.logging.debug("Backing up ipv6 rules...")
            filename = self.config.get_value("BACKUP", "ipv6filename")
            open(filepath + "/" + filename, 'w')
            self.save_execute(self.ip6tables_bin_save, filepath, filename)

    def save_execute(self, binary, filepath, filename):
        self.system_call(
            binary + " | while read IN ; do echo $IN >> " + filepath + "/" +
            filename + " ; done")

    def restore(self):
        log.logging.debug("Starting Firewall Rule Restore...")
        filepath = self.config.get_value("BACKUP", "filepath")
        utility.create_folder_if_not_exists(filepath)

        log.logging.debug("Restoring ipv4 rules...")
        filename = self.config.get_value("BACKUP", "ipv4filename")
        self.system_call(self.iptables_bin_restore + " < " +
                         filepath + "/" + filename)

        if self.ipv6 is True:
            log.logging.debug("Restoring ipv6 rules...")
            filename = self.config.get_value("BACKUP", "ipv6filename")
            self.system_call(self.ip6tables_bin_restore + " < " +
                             filepath + "/" + filename)

    @classmethod
    def system_call(self, command):
        # add security checks here
        os.system(command)
