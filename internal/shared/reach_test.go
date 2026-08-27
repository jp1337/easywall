package shared

import (
	"net/netip"
	"regexp"
	"testing"
)

func TestReachable(t *testing.T) {
	t.Parallel()
	const webPort = 12227

	lan := netip.MustParseAddr("192.168.133.234")
	pub := netip.MustParseAddr("203.0.113.7")
	v6 := netip.MustParseAddr("2001:db8::5")

	open := Rules{TCP: []PortRule{{Port: "12227", Description: "easywall"}}}

	cases := []struct {
		name    string
		rules   Rules
		opts    FirewallOptions
		net     NetworkSettings
		src     netip.Addr
		proxied bool
		local   bool
		verdict ReachVerdict
		reason  ReachReason
	}{
		{name: "a proxy peer decides nothing else",
			rules: open, src: netip.MustParseAddr("172.16.4.1"), proxied: true,
			verdict: ReachUnknown, reason: ReasonProxied},
		{name: "loopback is accepted first",
			src: netip.MustParseAddr("127.0.0.1"), verdict: ReachOpen, reason: ReasonLoopback},
		{name: "a non-loopback address the host holds itself is accepted like loopback",
			// Routed over lo regardless of which address it names; the caller
			// supplies this because it can enumerate the host's interfaces and
			// this package deliberately cannot.
			src: netip.MustParseAddr("192.168.1.5"), local: true,
			verdict: ReachOpen, reason: ReasonLoopback},
		{name: "the same address without local still gets a real answer",
			src: netip.MustParseAddr("192.168.1.5"), local: false,
			verdict: ReachBlocked, reason: ReasonNoRule},
		{name: "a zoned link-local address still matches a whitelist entry",
			// netip.Addr equality and Prefix.Contains both refuse a zoned
			// address outright; the zone has to come off before either runs.
			rules:   Rules{Whitelist: []string{"fe80::1"}},
			src:     netip.MustParseAddr("fe80::1%eth0"),
			verdict: ReachOpen, reason: ReasonWhitelisted},
		{name: "ipv6 passthrough accepts before anything else",
			net: NetworkSettings{IPv6: IPv6Config{Mode: IPv6Passthrough}}, src: v6,
			verdict: ReachOpen, reason: ReasonIPv6Passthrough},
		{name: "ipv6 block drops before anything else",
			rules: open, net: NetworkSettings{IPv6: IPv6Config{Mode: IPv6Block}}, src: v6,
			verdict: ReachBlocked, reason: ReasonIPv6Blocked},
		{name: "the bogon filter cannot be decided from here",
			rules: open, opts: FirewallOptions{Bogons: true}, src: lan,
			verdict: ReachUnknown, reason: ReasonBogonFilter},
		{name: "a whitelisted source leaves the bogon chain before any drop",
			rules: Rules{Whitelist: []string{"192.168.133.0/24"}},
			opts:  FirewallOptions{Bogons: true}, src: lan,
			verdict: ReachOpen, reason: ReasonWhitelisted},
		{name: "a named docker network is accepted",
			net: NetworkSettings{Docker: DockerConfig{Enabled: true,
				CustomNetworks: []string{"172.20.0.0/16"}}},
			src:     netip.MustParseAddr("172.20.0.9"),
			verdict: ReachOpen, reason: ReasonDockerNetwork},
		{name: "an auto-detected bridge is unknowable from here",
			net: NetworkSettings{Docker: DockerConfig{Enabled: true, AllowBridgeNetworks: true}},
			src: netip.MustParseAddr("172.18.0.4"),
			// The bogon filter is off here, so the docker step is what decides. A
			// distinct reason from the configured-network case above: this one
			// genuinely cannot be listed from here, and the sentence must not claim
			// otherwise the way the shared reason used to.
			verdict: ReachUnknown, reason: ReasonDockerBridge},
		{name: "an ordinary LAN address outside docker's pool still gets a real answer",
			// dockerPoolRanges bounds the unknowable case to 172.16.0.0/12; a
			// fallback-pool bridge address (192.168.x) is the known miss, but an
			// address nowhere near either must not be swept into the shrug too.
			net:     NetworkSettings{Docker: DockerConfig{Enabled: true, AllowBridgeNetworks: true}},
			src:     netip.MustParseAddr("10.0.0.5"),
			verdict: ReachBlocked, reason: ReasonNoRule},
		{name: "blacklist is consulted before whitelist",
			rules: Rules{Blacklist: []string{"203.0.113.7"}, Whitelist: []string{"203.0.113.7"}},
			src:   pub, verdict: ReachBlocked, reason: ReasonBlacklisted},
		{name: "a comment in the blacklist blocks nobody",
			rules: Rules{Blacklist: []string{"# 203.0.113.7 was noisy last week"}, TCP: open.TCP},
			src:   pub, verdict: ReachOpen, reason: ReasonPortOpen},
		{name: "the port being open is the ordinary answer",
			rules: open, src: pub, verdict: ReachOpen, reason: ReasonPortOpen},
		{name: "a range that contains the port counts",
			rules: Rules{TCP: []PortRule{{Port: "12000:13000"}}}, src: pub,
			verdict: ReachOpen, reason: ReasonPortOpen},
		{name: "a udp rule does not open a tcp port",
			rules: Rules{UDP: []PortRule{{Port: "12227"}}}, src: pub,
			verdict: ReachBlocked, reason: ReasonNoRule},
		{name: "custom rules run after everything and can accept",
			rules: Rules{Custom: []string{"tcp dport 12227 accept"}}, src: pub,
			verdict: ReachUnknown, reason: ReasonCustomRules},
		{name: "a custom block of nothing but comments decides nothing",
			rules: Rules{Custom: []string{"# nothing here yet", ""}}, src: pub,
			verdict: ReachBlocked, reason: ReasonNoRule},
		{name: "nothing matched and the policy drops",
			src: pub, verdict: ReachBlocked, reason: ReasonNoRule},
		{name: "an address that will not parse says so",
			src: netip.Addr{}, verdict: ReachUnknown, reason: ReasonNoAddress},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, reason := Reachable(tc.rules, tc.opts, tc.net, tc.src, webPort, tc.proxied, tc.local)
			if v != tc.verdict || reason != tc.reason {
				t.Errorf("got %s/%s, want %s/%s", v, reason, tc.verdict, tc.reason)
			}
		})
	}
}

// portInRule is the one place an off-by-one would hide: the table test above
// exercises a range rule only at 12227, comfortably inside 12000:13000, and
// would still pass with either boundary wrong by one.
func TestPortInRule(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		spec string
		port uint16
		want bool
	}{
		{"exact match", "12227", 12227, true},
		{"exact mismatch", "12227", 12228, false},
		{"range low boundary is inclusive", "12000:13000", 12000, true},
		{"range high boundary is inclusive", "12000:13000", 13000, true},
		{"just below the range misses", "12000:13000", 11999, false},
		{"just above the range misses", "12000:13000", 13001, false},
		{"a range missing its high end opens nothing", "12000:", 12227, false},
		{"text that is not a port opens nothing", "abc", 12227, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := portInRule(tc.spec, tc.port); got != tc.want {
				t.Errorf("portInRule(%q, %d) = %v, want %v", tc.spec, tc.port, got, tc.want)
			}
		})
	}
}

// Every reason a verdict can carry is declared in AllReachReasons: the web layer
// derives its locale keys from that list, so one missing from it is a sentence
// that renders as its own message id on the apply screen.
//
// The list this used to check against was a second hand-written copy of the
// same names, in this file — so a ReasonFoo added to the const block in
// reach.go and forgotten in AllReachReasons passed here, passed
// TestEveryReachReasonHasALabel (which iterates AllReachReasons, not the
// source), and rendered as the literal "reach_foo" on the page. This scans
// reach.go's actual const declarations instead, the way
// TestAllCommandTypesMatchesTheProtocolSource cross-checks protocol.go against
// AllCommandTypes.
func TestAllReachReasonsIsComplete(t *testing.T) {
	t.Parallel()
	reachSource := repoFile(t, "internal", "shared", "reach.go")

	constPattern := regexp.MustCompile(`(Reason\w+)\s+ReachReason\s*=\s*"([^"]+)"`)
	matches := constPattern.FindAllStringSubmatch(reachSource, -1)
	if len(matches) == 0 {
		t.Fatal("could not find any ReachReason constants in reach.go; " +
			"the pattern no longer matches or the file is missing declarations")
	}

	declaredValues := make(map[string]string) // value -> name
	for _, m := range matches {
		declaredValues[m[2]] = m[1]
	}

	declared := map[ReachReason]bool{}
	for _, r := range AllReachReasons {
		if declared[r] {
			t.Errorf("%q is listed in AllReachReasons twice", r)
		}
		declared[r] = true
	}

	// Every constant reach.go declares must be listed in AllReachReasons.
	for value, name := range declaredValues {
		if !declared[ReachReason(value)] {
			t.Errorf("%s (value %q) is declared in reach.go but missing from AllReachReasons", name, value)
		}
	}

	// Every entry in AllReachReasons must correspond to a declared constant.
	for _, r := range AllReachReasons {
		if _, ok := declaredValues[string(r)]; !ok {
			t.Errorf("AllReachReasons lists %q but no constant with that value is declared in reach.go", string(r))
		}
	}

	if len(AllReachReasons) != len(matches) {
		t.Errorf("AllReachReasons has %d entries but reach.go declares %d ReachReason constants",
			len(AllReachReasons), len(matches))
	}
}

func TestReachable_PortSources(t *testing.T) {
	const port = 12227
	lan := netip.MustParseAddr("192.168.1.50")
	outside := netip.MustParseAddr("203.0.113.9")

	cases := []struct {
		name        string
		src         netip.Addr
		sources     []string
		wantVerdict ReachVerdict
		wantReason  ReachReason
	}{
		{name: "no sources is anywhere", src: outside, sources: nil,
			wantVerdict: ReachOpen, wantReason: ReasonPortOpen},
		{name: "inside the restriction", src: lan, sources: []string{"192.168.0.0/16"},
			wantVerdict: ReachOpen, wantReason: ReasonPortOpen},
		{name: "outside the restriction", src: outside, sources: []string{"192.168.0.0/16"},
			wantVerdict: ReachBlocked, wantReason: ReasonPortSourceMismatch},
		{name: "a bare address in the restriction", src: lan, sources: []string{"192.168.1.50"},
			wantVerdict: ReachOpen, wantReason: ReasonPortOpen},
		{name: "comments alone restrict to nobody", src: lan, sources: []string{"# todo"},
			wantVerdict: ReachBlocked, wantReason: ReasonPortSourceMismatch},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Rules{TCP: []PortRule{{Port: "12227", Sources: tc.sources}}}
			v, reason := Reachable(r, FirewallOptions{}, NetworkSettings{}, tc.src, port, false, false)
			if v != tc.wantVerdict || reason != tc.wantReason {
				t.Errorf("Reachable = (%s, %s), want (%s, %s)", v, reason, tc.wantVerdict, tc.wantReason)
			}
		})
	}
}

// A second rule for the same port, unrestricted, opens it: the chain accepts on
// the first rule that matches, and a blocked verdict from an earlier restricted
// rule would be a warning nobody could act on.
func TestReachable_PortSources_AnUnrestrictedRuleWins(t *testing.T) {
	src := netip.MustParseAddr("203.0.113.9")
	r := Rules{TCP: []PortRule{
		{Port: "443", Sources: []string{"192.168.0.0/16"}},
		{Port: "443"},
	}}
	v, reason := Reachable(r, FirewallOptions{}, NetworkSettings{}, src, 443, false, false)
	if v != ReachOpen || reason != ReasonPortOpen {
		t.Errorf("Reachable = (%s, %s), want (open, port_open)", v, reason)
	}
}

// A custom rule can accept exactly the traffic a port-source restriction turned
// away, and the nft CLI appends custom rules after everything netlink wrote —
// so a custom rule outranks the restriction. Calling that combination blocked
// would be a false lockout warning on the one screen that has to be believed.
func TestReachable_PortSources_ACustomRuleOutranksTheRestriction(t *testing.T) {
	src := netip.MustParseAddr("203.0.113.9")
	r := Rules{
		TCP:    []PortRule{{Port: "443", Sources: []string{"192.168.0.0/16"}}},
		Custom: []string{"tcp dport 443 accept"},
	}
	v, reason := Reachable(r, FirewallOptions{}, NetworkSettings{}, src, 443, false, false)
	if v != ReachUnknown || reason != ReasonCustomRules {
		t.Errorf("Reachable = (%s, %s), want (unknown, custom_rules)", v, reason)
	}
}

// Two restricted rules for the same port: the first excludes the caller, the
// second covers it. A verdict that only ever looks at the first restricted
// rule would call this blocked, but the kernel evaluates every rule for the
// port in order and accepts on the second one's match.
func TestReachable_PortSources_ASecondRestrictedRuleCanCoverTheCaller(t *testing.T) {
	src := netip.MustParseAddr("203.0.113.9")
	r := Rules{TCP: []PortRule{
		{Port: "443", Sources: []string{"192.168.0.0/16"}},
		{Port: "443", Sources: []string{"203.0.113.0/24"}},
	}}
	v, reason := Reachable(r, FirewallOptions{}, NetworkSettings{}, src, 443, false, false)
	if v != ReachOpen || reason != ReasonPortOpen {
		t.Errorf("Reachable = (%s, %s), want (open, port_open)", v, reason)
	}
}
