package core

import (
	"encoding/binary"
	"log/slog"
	"net"
	"net/netip"
	"strings"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"github.com/jp1337/easywall/internal/shared"
	"golang.org/x/sys/unix"
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

// publishedPort is one container port Docker has DNAT'd on to this host: the
// address it is published on, the port, and the protocol.
type publishedPort struct {
	addr  string
	port  uint16
	proto string // "tcp", "udp", or "" when the rule did not say
}

// detectPublishedPortsFn is where the published ports come from. A var for the
// same reason netInterfacesFn is one: the real implementation needs a netlink
// socket and CAP_NET_ADMIN, and the comparison it feeds has to be testable
// without either.
var detectPublishedPortsFn = detectPublishedPorts

// dockerNATTables are the tables a published port's DNAT rule is written into:
// `ip nat` with the iptables-nft backend, `ip docker` with the native nftables
// one. Chains are not named here — Docker has renamed those more than once, and
// a rule is identified below by what it does, not by where it sits.
var dockerNATTables = map[string]bool{"nat": true, "docker": true}

// detectPublishedPorts returns the container ports Docker has published on this
// host, as the kernel has them.
//
// It reads Docker's own DNAT rules rather than asking the Docker daemon: the
// same place the bridge detection above looks — what is actually programmed —
// and it needs no socket, no client library and no running daemon. A host whose
// Docker has stopped with its rules still loaded is exactly the host this
// warning is about.
//
// cidrs are the bridge networks the caller already detected, and they are the
// filter: only a DNAT whose *target* lands inside one of them is a published
// container port. Every other DNAT on the machine — an operator's own port
// forward, a load balancer's — is somebody else's rule and is not what the
// forward chain's deny can close.
//
// Its own netlink connection, deliberately: NftablesManager's is mid-transaction
// when this runs, and a dump interleaved into a batch is not a risk worth the
// one socket it saves.
//
// Anything it cannot read is nothing. This produces a log line, not a verdict,
// so failing quiet is right — a daemon that refused to apply because it could
// not enumerate Docker's NAT rules would be a far worse bug than the one this
// warns about.
func detectPublishedPorts(cidrs []string) []publishedPort {
	var nets []netip.Prefix
	for _, c := range cidrs {
		if shared.IsListComment(c) {
			continue
		}
		if p, err := netip.ParsePrefix(strings.TrimSpace(c)); err == nil {
			nets = append(nets, p)
		}
	}
	if len(nets) == 0 {
		return nil
	}

	conn, err := nftables.New()
	if err != nil {
		slog.Debug("published port detection: no netlink connection", "error", err)
		return nil
	}
	tables, err := conn.ListTablesOfFamily(nftables.TableFamilyIPv4)
	if err != nil {
		slog.Debug("published port detection: cannot list tables", "error", err)
		return nil
	}
	chains, err := conn.ListChainsOfTableFamily(nftables.TableFamilyIPv4)
	if err != nil {
		slog.Debug("published port detection: cannot list chains", "error", err)
		return nil
	}

	var out []publishedPort
	for _, tbl := range tables {
		if !dockerNATTables[tbl.Name] {
			continue
		}
		for _, ch := range chains {
			if ch.Table == nil || ch.Table.Name != tbl.Name || ch.Table.Family != tbl.Family {
				continue
			}
			rules, err := conn.GetRules(tbl, ch)
			if err != nil {
				continue
			}
			for _, r := range rules {
				if p, ok := publishedPortFromRule(r, nets); ok {
					out = append(out, p)
				}
			}
		}
	}
	return out
}

// publishedPortFromRule reads one kernel rule as a published port, and reports
// false for everything that is not one.
//
// What it is looking for is a destination NAT whose target address is inside one
// of nets, and the destination port that reaches it:
//
//	ip daddr 172.17.0.1 meta l4proto udp udp dport 53 dnat to 172.18.0.2:53
//
// A rule with no `ip daddr` is published on every address, which is what
// `-p 53:53` means and what 0.0.0.0 says here.
//
// Deliberately forgiving in one direction only. A comparison it cannot read
// leaves that field unset and the rule is skipped if the field was required, so
// an unfamiliar rule shape produces no warning rather than a wrong one. The
// payload it last saw is cleared by anything that is not the comparison against
// it — a bitwise mask in between means `ip daddr 10.0.0.0/8`, a network and not
// the single address this is about.
func publishedPortFromRule(r *nftables.Rule, nets []netip.Prefix) (publishedPort, bool) {
	p := publishedPort{addr: "0.0.0.0"}
	immediates := map[uint32][]byte{}
	var lastPayload *expr.Payload
	var lastMeta *expr.Meta
	var nat *expr.NAT

	for _, e := range r.Exprs {
		payload, meta := lastPayload, lastMeta
		lastPayload, lastMeta = nil, nil

		switch v := e.(type) {
		case *expr.Payload:
			lastPayload = v
		case *expr.Meta:
			lastMeta = v
		case *expr.Immediate:
			immediates[v.Register] = v.Data
		case *expr.NAT:
			nat = v
		case *expr.Cmp:
			if v.Op != expr.CmpOpEq {
				continue
			}
			switch {
			case meta != nil && meta.Key == expr.MetaKeyL4PROTO && len(v.Data) == 1:
				p.proto = protoName(v.Data[0])
			case payload == nil:
			case payload.Base == expr.PayloadBaseNetworkHeader &&
				payload.Offset == 16 && payload.Len == 4 && len(v.Data) == 4:
				p.addr = net.IP(v.Data).String()
			case payload.Base == expr.PayloadBaseNetworkHeader &&
				payload.Offset == 9 && payload.Len == 1 && len(v.Data) == 1:
				p.proto = protoName(v.Data[0])
			case payload.Base == expr.PayloadBaseTransportHeader &&
				payload.Offset == 2 && payload.Len == 2 && len(v.Data) == 2:
				p.port = binary.BigEndian.Uint16(v.Data)
			}
		}
	}

	if nat == nil || nat.Type != expr.NATTypeDestNAT || p.port == 0 {
		return publishedPort{}, false
	}
	target, ok := netip.AddrFromSlice(immediates[nat.RegAddrMin])
	if !ok {
		return publishedPort{}, false
	}
	for _, n := range nets {
		if n.Contains(target) {
			return p, true
		}
	}
	return publishedPort{}, false
}

// protoName names the two transport protocols a published port can use, and
// nothing else — an unknown number leaves the protocol unsaid, which the
// comparison reads as "check both".
func protoName(n byte) string {
	switch n {
	case unix.IPPROTO_TCP:
		return "tcp"
	case unix.IPPROTO_UDP:
		return "udp"
	default:
		return ""
	}
}
