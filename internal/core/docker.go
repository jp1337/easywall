package core

import (
	"log/slog"
	"net"
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
// Detection lists the host's network interfaces, keeps the ones named docker*
// or br-, and takes the IPv4 network of each address they carry. The comment
// here used to describe reading /proc/net/fib_trie, which nothing in this file
// has ever opened.
//
// IPv4 only. A Docker network with IPv6 enabled is not detected, and has to go
// in docker.custom_networks — see features/docker.md.
//
// The name test is a prefix match, so any bridge called br-something counts,
// Docker's or not. That is deliberate on a container host and worth knowing on
// a router, where br-lan would also be accepted.
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
