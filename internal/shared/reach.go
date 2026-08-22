package shared

import (
	"net/netip"
	"strconv"
	"strings"
)

// Whether a new connection from one address can still reach a port once the
// staged rules are live.
//
// Not "will I be disconnected". Flushing and rebuilding the table does not touch
// conntrack: the operator's browser connection stays ESTABLISHED, matches the
// established/related accept, and keeps answering after an apply that closes the
// web port entirely. So the page can go on working while the firewall no longer
// admits anyone, and confirming from it would confirm a lockout. This function
// answers the question that is actually useful, and the copy on the page says
// what it means.
//
// The chain order below is nft.Apply's, duplicated here by construction. The
// coupling meant to keep the two honest is a veth test in internal/core, not
// yet in this tree — the next task after this one: a rule inserted into Apply
// without a branch here should show up as a disagreement with a real kernel
// rather than as a wrong warning on a page.

// ReachVerdict is the answer. Three values, because there are three honest ones.
type ReachVerdict string

const (
	ReachOpen    ReachVerdict = "open"
	ReachBlocked ReachVerdict = "blocked"
	ReachUnknown ReachVerdict = "unknown"
)

// ReachReason names which rule decided. A closed enum, never free text, for the
// same reason LoginEvent is one: this ends up on a page and in two locale files,
// and a sentence assembled in Go cannot be translated.
type ReachReason string

const (
	ReasonNoAddress       ReachReason = "no_address"
	ReasonProxied         ReachReason = "proxied"
	ReasonLoopback        ReachReason = "loopback"
	ReasonIPv6Passthrough ReachReason = "ipv6_passthrough"
	ReasonIPv6Blocked     ReachReason = "ipv6_blocked"
	ReasonBogonFilter     ReachReason = "bogon_filter"
	ReasonDockerNetwork   ReachReason = "docker_network"
	ReasonBlacklisted     ReachReason = "blacklisted"
	ReasonWhitelisted     ReachReason = "whitelisted"
	ReasonPortOpen        ReachReason = "port_open"
	ReasonCustomRules     ReachReason = "custom_rules"
	ReasonNoRule          ReachReason = "no_rule"
)

// AllReachReasons is the complete list, and it is what the interface's guard
// hangs off: both locale files must label every one of these.
var AllReachReasons = []ReachReason{
	ReasonNoAddress, ReasonProxied, ReasonLoopback, ReasonIPv6Passthrough,
	ReasonIPv6Blocked, ReasonBogonFilter, ReasonDockerNetwork, ReasonBlacklisted,
	ReasonWhitelisted, ReasonPortOpen, ReasonCustomRules, ReasonNoRule,
}

// BogonRanges are the source networks the bogon filter drops on any interface
// but loopback: "impossible" sources on the public internet.
//
// One list, two readers — core's addBogonFilter builds the rules from it and
// Reachable reasons about it. filters.md documents the same eleven, and the
// three used to be maintained by hand: the page listed "this network" and
// loopback when neither was in the code, while TEST-NET-3 and the reserved space
// were in the code and not on the page.
//
// IPv4 only, deliberately: fe80::/10 is link-local and IPv6 needs neighbour
// discovery on it to work at all, so the IPv6 equivalents are not a symmetric
// translation.
var BogonRanges = []string{
	"0.0.0.0/8",       // "this network"
	"10.0.0.0/8",      // private
	"100.64.0.0/10",   // carrier-grade NAT
	"127.0.0.0/8",     // loopback, which cannot arrive on a real interface
	"169.254.0.0/16",  // link-local
	"172.16.0.0/12",   // private
	"192.0.2.0/24",    // TEST-NET-1
	"192.168.0.0/16",  // private
	"198.51.100.0/24", // TEST-NET-2
	"203.0.113.0/24",  // TEST-NET-3
	"240.0.0.0/4",     // reserved
}

// dockerPoolRanges bounds the one case the web process genuinely cannot decide
// — see the docker step in Reachable. It is deliberately narrower than Docker's
// full predefined pool: libnetwork's local-scope pools are 172.17.0.0/16
// through 172.31.0.0/16 and then /20 slices of 192.168.0.0/16 once the 172
// space is exhausted, but including the 192.168 fallback here would make every
// ordinary LAN address in that range unknown, which is the shrug the bogon-step
// comment above already refuses. The trade-off is accepted knowingly: a
// fallback-pool bridge address is the one auto-detected-Docker case this
// function still gets wrong, and it is a false "blocked", never a false
// "reachable".
var dockerPoolRanges = []string{"172.16.0.0/12"}

// Reachable reads the input chain in the order nft.Apply writes it and stops at
// the first rule that decides. proxied comes from the *presence* of a
// forwarding header on the request, never its value. local means "this address
// is one the host itself holds" — computed by the caller from
// net.InterfaceAddrs(), which stays out of this package deliberately: this
// function reasons about rules, not about which interfaces exist on the machine
// it happens to run on.
func Reachable(r Rules, o FirewallOptions, n NetworkSettings,
	src netip.Addr, port uint16, proxied, local bool) (ReachVerdict, ReachReason) {

	// Earlier than the chain, because no amount of rule evaluation fixes it: if
	// the peer is a proxy then src is the proxy's address and not the operator's.
	if proxied {
		return ReachUnknown, ReasonProxied
	}
	// Unmap first, so an IPv4-mapped IPv6 address is treated as the IPv4 address
	// it is; strip the zone next, because a zoned link-local address such as
	// fe80::1%eth0 is otherwise equal to nothing in an operator's list — netip.Addr
	// equality includes the zone, and Prefix.Contains refuses a zoned address
	// outright.
	src = src.Unmap().WithZone("")
	if !src.IsValid() {
		return ReachUnknown, ReasonNoAddress
	}

	// 1. loopback accept, and the kernel decides it on the arrival interface
	// rather than on the address: a connection to *any* address this host holds
	// is routed over lo and accepted before a rule is consulted. src.IsLoopback()
	// catches only the 127.0.0.0/8 and ::1 spelling of that, so the caller —
	// which can enumerate this host's interfaces and this package deliberately
	// cannot — supplies the rest.
	if local || src.IsLoopback() {
		return ReachOpen, ReasonLoopback
	}

	// 2. The IPv6 disposition, immediately after loopback: passthrough and block
	// are statements about all IPv6 traffic, and a later rule would only see what
	// they left.
	if src.Is6() {
		switch n.IPv6.Mode {
		case IPv6Passthrough:
			return ReachOpen, ReasonIPv6Passthrough
		case IPv6Block:
			return ReachBlocked, ReasonIPv6Blocked
		}
	}

	// 3. established/related is not consulted: a new connection is not
	// established, and that is the whole distinction this function is built on.
	// 4. ICMP is irrelevant to a TCP connection.

	// The Docker networks the web process can actually name. The auto-detected
	// bridges are settled in the core at apply time and are not knowable here.
	var dockerNets []string
	if n.Docker.Enabled {
		dockerNets = n.Docker.CustomNetworks
	}

	// 5. The protection modules. Rate limits are not a verdict — a limit decides
	// how much, not whether. The bogon filter is different, and it is unknown
	// rather than blocked: the rule matches on the arrival interface, which this
	// process cannot know, and easywall's own audience reaches the interface from
	// exactly the RFC-1918 addresses it drops. Guessing either way would produce
	// the two worst outcomes available — a false alarm on every LAN request, or
	// silence on a real lockout.
	if o.Bogons && src.Is4() && inAnyCIDR(src, BogonRanges) &&
		!inAnyEntry(src, r.Whitelist) && !inAnyCIDR(src, dockerNets) {
		return ReachUnknown, ReasonBogonFilter
	}

	// 6. The Docker bridge accepts.
	if n.Docker.Enabled {
		if inAnyCIDR(src, dockerNets) {
			return ReachOpen, ReasonDockerNetwork
		}
		// An auto-detected bridge could accept this and this process cannot list
		// them. Bounded to Docker's default address pool so an ordinary 192.168 or
		// 10.x LAN address still gets a real answer instead of a shrug.
		if n.Docker.AllowBridgeNetworks && inAnyCIDR(src, dockerPoolRanges) {
			return ReachUnknown, ReasonDockerNetwork
		}
	}

	// 7. The blacklist drops, and it is consulted *before* the whitelist. An
	// operator's own address on both lists is blocked. That is the trap.
	if inAnyEntry(src, r.Blacklist) {
		return ReachBlocked, ReasonBlacklisted
	}

	// 8. The whitelist accepts.
	if inAnyEntry(src, r.Whitelist) {
		return ReachOpen, ReasonWhitelisted
	}

	// 9. The port. TCP only: the interface is served over TCP, and a UDP rule for
	// the same number opens nothing for it.
	for _, rule := range r.TCP {
		if portInRule(rule.Port, port) {
			return ReachOpen, ReasonPortOpen
		}
	}

	// 10. The custom rules, which the nft CLI appends after everything netlink
	// wrote. Parsing raw nftables expressions to find out what they do is not
	// something to attempt on a page that has to be believed.
	for _, line := range r.Custom {
		if !IsListComment(line) {
			return ReachUnknown, ReasonCustomRules
		}
	}

	// The policy.
	return ReachBlocked, ReasonNoRule
}

// inAnyEntry reports whether src is covered by an operator-written list entry —
// a bare address or a network, with comments and blanks skipped, exactly as the
// rule builders skip them.
func inAnyEntry(src netip.Addr, entries []string) bool {
	for _, entry := range entries {
		if IsListComment(entry) {
			continue
		}
		e := strings.TrimSpace(entry)
		if addr, err := netip.ParseAddr(e); err == nil {
			if addr.Unmap() == src {
				return true
			}
			continue
		}
		if pfx, err := netip.ParsePrefix(e); err == nil && pfx.Contains(src) {
			return true
		}
	}
	return false
}

// inAnyCIDR is inAnyEntry for lists that hold networks only.
func inAnyCIDR(src netip.Addr, cidrs []string) bool {
	for _, c := range cidrs {
		if IsListComment(c) {
			continue
		}
		if pfx, err := netip.ParsePrefix(strings.TrimSpace(c)); err == nil && pfx.Contains(src) {
			return true
		}
	}
	return false
}

// portInRule reports whether a port rule covers port. The stored form is a
// single number or a "low:high" range, which is what the ports editor writes and
// what buildPortExprs turns into rules.
func portInRule(spec string, port uint16) bool {
	spec = strings.TrimSpace(spec)
	low, high, isRange := strings.Cut(spec, ":")
	lo, err := strconv.ParseUint(strings.TrimSpace(low), 10, 16)
	if err != nil {
		return false
	}
	if !isRange {
		return uint16(lo) == port
	}
	hi, err := strconv.ParseUint(strings.TrimSpace(high), 10, 16)
	if err != nil {
		return false
	}
	return uint16(lo) <= port && port <= uint16(hi)
}
