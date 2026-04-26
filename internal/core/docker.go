package core

import (
	"bufio"
	"log/slog"
	"net"
	"os"
	"strings"
)

// netInterfacesFn is the function used to list network interfaces; overridden in tests.
var netInterfacesFn = net.Interfaces

// ifaceAddrsFn returns the addresses of a network interface; overridden in tests.
var ifaceAddrsFn = func(iface net.Interface) ([]net.Addr, error) {
	return iface.Addrs()
}

// detectDockerBridges returns CIDR ranges for all Docker bridge networks
// currently active on the system (e.g. "172.17.0.0/16").
//
// Detection works by reading /proc/net/fib_trie and cross-referencing with
// network interfaces whose names start with "docker" or "br-".
func detectDockerBridges() []string {
	interfaces, err := netInterfacesFn()
	if err != nil {
		slog.Warn("docker bridge detection: cannot list interfaces", "error", err)
		return nil
	}

	var cidrs []string
	for _, iface := range interfaces {
		if !isDockerInterface(iface.Name) {
			continue
		}

		addrs, err := ifaceAddrsFn(iface)
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				if v.IP.To4() != nil {
					network := &net.IPNet{IP: v.IP.Mask(v.Mask), Mask: v.Mask}
					cidrs = append(cidrs, network.String())
				}
			}
		}
	}

	return cidrs
}

// isDockerInterface returns true if the interface name looks like a Docker
// bridge (docker0, docker_gwbridge, br-<hash>).
func isDockerInterface(name string) bool {
	return strings.HasPrefix(name, "docker") ||
		strings.HasPrefix(name, "br-")
}

// dockerSocketPaths lists Unix socket paths checked to detect Docker; overridden in tests.
var dockerSocketPaths = []string{"/var/run/docker.sock", "/run/docker.sock"}

// procNetDevPath is the path to /proc/net/dev; overridden in tests.
var procNetDevPath = "/proc/net/dev"

// isDockerRunning returns true when the Docker socket exists, indicating
// that Docker is likely running on this host.
func isDockerRunning() bool {
	for _, sock := range dockerSocketPaths {
		if _, err := os.Stat(sock); err == nil {
			return true
		}
	}
	return false
}

// readProcNetDev returns the list of network interface names from /proc/net/dev.
// Used as a fallback when net.Interfaces() is unavailable.
func readProcNetDev() []string {
	f, err := os.Open(procNetDevPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var names []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if idx := strings.Index(line, ":"); idx > 0 {
			names = append(names, strings.TrimSpace(line[:idx]))
		}
	}
	return names
}
