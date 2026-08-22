package shared

import (
	"net/netip"
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
		verdict ReachVerdict
		reason  ReachReason
	}{
		{name: "a proxy peer decides nothing else",
			rules: open, src: netip.MustParseAddr("172.16.4.1"), proxied: true,
			verdict: ReachUnknown, reason: ReasonProxied},
		{name: "loopback is accepted first",
			src: netip.MustParseAddr("127.0.0.1"), verdict: ReachOpen, reason: ReasonLoopback},
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
			// The bogon filter is off here, so the docker step is what decides.
			verdict: ReachUnknown, reason: ReasonDockerNetwork},
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
			v, reason := Reachable(tc.rules, tc.opts, tc.net, tc.src, webPort, tc.proxied)
			if v != tc.verdict || reason != tc.reason {
				t.Errorf("got %s/%s, want %s/%s", v, reason, tc.verdict, tc.reason)
			}
		})
	}
}

// Every reason a verdict can carry is declared in AllReachReasons: the web layer
// derives its locale keys from that list, so one missing from it is a sentence
// that renders as its own message id on the apply screen.
func TestAllReachReasonsIsComplete(t *testing.T) {
	t.Parallel()
	declared := map[ReachReason]bool{}
	for _, r := range AllReachReasons {
		if declared[r] {
			t.Errorf("%q is listed in AllReachReasons twice", r)
		}
		declared[r] = true
	}
	for _, r := range []ReachReason{
		ReasonNoAddress, ReasonProxied, ReasonLoopback, ReasonIPv6Passthrough,
		ReasonIPv6Blocked, ReasonBogonFilter, ReasonDockerNetwork,
		ReasonBlacklisted, ReasonWhitelisted, ReasonPortOpen, ReasonCustomRules,
		ReasonNoRule,
	} {
		if !declared[r] {
			t.Errorf("%q is declared but missing from AllReachReasons", r)
		}
	}
}
