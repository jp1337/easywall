import config
import log
import iptables
import acceptance


class easywall(object):
    def __init__(self):
        self.log = log.log()
        log.logging.info("Starting up EasyWall...")
        self.iptables = iptables.iptables()
        self.acceptance = acceptance.acceptance()
        self.apply()  # only testing
        log.logging.info("Shutting down EasyWall...")
        self.log.closeLogging()

    def apply(self):
        self.acceptance.reset()
        self.iptables.save()
        self.iptables.reset()
        # drop incoming connections && accept outgoing connections
        # allow loopback access
        # allow established or related connections
        # Block remote packets claiming to be from a loopback address.
        # Block IP-addresses from blacklist
        # Allow IP-addresses from whitelist
        # Allow TCP Ports
        # Allow UDP Ports
        # Drop and Log invalid packages
        # Final Rules
        self.iptables.list()
        if self.acceptance.check() == False:
            self.iptables.restore()
        else:
            # move old rules
            # save new rules
            print("")
        self.iptables.list()


if __name__ == "__main__":
    easywall()
