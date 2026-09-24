//go:build integration

package core

import (
	"errors"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// Three modules that reported themselves on in v2.21.1 and did next to nothing,
// measured with real packets over the router harness's veth: 10.77.1.2 in the
// namespace on side A reaches this host at 10.77.1.1.
//
// Every case runs a control first — the same traffic with the module off — so a
// harness that does not route cannot pass for a module that drops.

// pingReceived pings to from the namespace on side A and returns how many
// replies came back. The exit status is ignored: ping exits 1 when any reply is
// lost, which is what half of these cases are about.
func pingReceived(t *testing.T, r *router, args ...string) int {
	t.Helper()
	cmd := exec.Command("nsenter", append([]string{"-t", r.pidA, "-n", "ping", "-q"}, args...)...)
	out, _ := cmd.CombinedOutput()
	m := regexp.MustCompile(`(\d+) received`).FindSubmatch(out)
	if m == nil {
		t.Fatalf("ping %v printed no summary:\n%s", args, out)
	}
	n, _ := strconv.Atoi(string(m[1]))
	return n
}

// ndAllowed is filter mode with neighbour discovery accepted, which is what the
// shipped configuration has: without it this host never learns side A's link
// address, and no IPv6 ping is answered with or without a module.
var ndAllowed = shared.NetworkSettings{IPv6: shared.IPv6Config{
	Mode: shared.IPv6Filter, ICMPAllowNeighborAdvertisement: true,
}}

func applyRules(t *testing.T, m *NftablesManager, rules shared.Rules, opts shared.FirewallOptions) {
	t.Helper()
	state := shared.RulesState{Current: rules, Staged: rules, Backup: rules}
	if err := m.Apply(state, opts, ndAllowed); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}

// The input chain never sees a fragment: ip_local_deliver reassembles before
// LOCAL_IN. So the rule has to sit at prerouting, ahead of conntrack's own
// reassembly at -400 — and this is the test that fails when it does not.
func TestIntegration_FragmentDropDropsFragments(t *testing.T) {
	m := newIntegrationManager(t)
	r := newRouter(t)
	// Both ends: a veth drops a frame longer than the receiving end's MTU, so
	// lowering one side alone loses the host's 1500-byte reply fragments.
	r.inNS(r.pidA, "ip", "link", "set", "va", "mtu", "1280")
	r.run("ip", "link", "set", "va-r", "mtu", "1280")

	// The whitelist, so an IPv4 echo request is accepted at all: type 8 is
	// not in the ICMP accept list.
	rules := shared.Rules{Whitelist: []string{"10.77.1.0/24"}}
	big := []string{"-c", "2", "-W", "1", "-M", "dont", "-s", "3000", "10.77.1.1"}
	small := []string{"-c", "2", "-W", "1", "10.77.1.1"}

	applyRules(t, m, rules, shared.FirewallOptions{})
	if got := pingReceived(t, r, big...); got != 2 {
		t.Fatalf("control: a 3000-byte ping over a 1280 MTU got %d of 2 replies with the "+
			"switch off; the harness is not delivering fragments, so nothing below means anything", got)
	}

	applyRules(t, m, rules, shared.FirewallOptions{Fragments: true})
	if got := pingReceived(t, r, big...); got != 0 {
		t.Errorf("a fragmented ping got %d of 2 replies with drop_fragments on\n"+
			"fragments chain:\n  %s", got, strings.Join(chainText(t, fragmentChainName), "\n  "))
	}
	if got := pingReceived(t, r, small...); got != 2 {
		t.Errorf("an unfragmented ping got %d of 2 replies with drop_fragments on; "+
			"the rule drops more than fragments", got)
	}

	// Only what is addressed to this host. With routing open the host passes
	// side A's fragments on to side B, and the switch must not start dropping
	// them: the input chain it used to sit in never saw routed traffic.
	r.inNS(r.pidB, "ip", "link", "set", "vb", "mtu", "1280")
	r.run("ip", "link", "set", "vb-r", "mtu", "1280")
	routed := shared.NetworkSettings{Routing: shared.RoutingConfig{Mode: shared.RoutingOpen}}
	state := shared.RulesState{Current: rules, Staged: rules, Backup: rules}
	if err := m.Apply(state, shared.FirewallOptions{Fragments: true}, routed); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := pingReceived(t, r, "-c", "2", "-W", "1", "-M", "dont", "-s", "3000", r.addrB); got != 2 {
		t.Errorf("a fragmented ping routed through this host got %d of 2 replies with "+
			"drop_fragments on; the switch reaches traffic that is not addressed here", got)
	}

	// Nor on loopback, which the input chain accepts before anything else.
	// Loopback's MTU is 65536 and in this namespace it starts down, so this
	// takes changing both, and putting them back for the tests that share it.
	wasUp := false
	if out, err := exec.Command("ip", "-o", "link", "show", "lo").Output(); err == nil {
		wasUp = strings.Contains(string(out), ",UP")
	}
	if out, err := exec.Command("ip", "link", "set", "lo", "up", "mtu", "1280").CombinedOutput(); err != nil {
		t.Fatalf("lower lo's MTU: %v: %s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("ip", "link", "set", "lo", "mtu", "65536").Run()
		if !wasUp {
			_ = exec.Command("ip", "link", "set", "lo", "down").Run()
		}
	})
	out, err := exec.Command("ping", "-q", "-c", "2", "-W", "1", "-M", "dont", "-s", "3000", "127.0.0.1").CombinedOutput()
	if err != nil || !strings.Contains(string(out), " 2 received") {
		t.Errorf("a fragmented ping over loopback was dropped with drop_fragments on:\n%s", out)
	}

	// Where it sits, in the kernel's own words, so a red run says why.
	out, err = exec.Command("nft", "list", "chain", "inet", tableName, fragmentChainName).CombinedOutput()
	if err != nil {
		t.Fatalf("nft list chain %s: %v\n%s", fragmentChainName, err, out)
	}
	for _, want := range []string{"hook prerouting priority -450", "policy accept"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("the fragments chain is not %q:\n%s", want, out)
		}
	}
}

// Both meters sit ahead of the established accept. Behind it they metered one
// packet per ping process — every echo request after the first is established —
// and one reset per nothing: a reset for a connection is established, and one
// for no connection is invalid.
func TestIntegration_ICMPFloodMetersEveryEchoRequest(t *testing.T) {
	m := newIntegrationManager(t)
	r := newRouter(t)
	r.run("ip", "-6", "addr", "add", "fd77:1::1/64", "dev", "va-r", "nodad")
	r.inNS(r.pidA, "ip", "-6", "addr", "add", "fd77:1::2/64", "dev", "va", "nodad")

	rules := shared.Rules{Whitelist: []string{"10.77.1.0/24"}}
	// Twenty in one second from one source, against a limit of one a second
	// with a burst of one: two get through at most.
	flood := func(to string) []string { return []string{"-c", "20", "-i", "0.05", "-W", "1", to} }

	applyRules(t, m, rules, shared.FirewallOptions{})
	for _, to := range []string{"10.77.1.1", "fd77:1::1"} {
		if got := pingReceived(t, r, flood(to)...); got < 18 {
			t.Fatalf("control: %d of 20 pings to %s answered with the module off; "+
				"the harness is dropping on its own", got, to)
		}
	}

	applyRules(t, m, rules, shared.FirewallOptions{ICMPFlood: true, ICMPFloodConnectionLimit: 1})
	for _, to := range []string{"10.77.1.1", "fd77:1::1"} {
		if got := pingReceived(t, r, flood(to)...); got > 4 {
			t.Errorf("%d of 20 pings to %s answered under a limit of 1/s\n"+
				"input chain:\n  %s", got, to, strings.Join(chainText(t, "input"), "\n  "))
		}
	}
}

// A reset this host receives for its own refused connection is the everyday
// case of a tracked reset, and the one a test can make: dial a closed port on
// side A, and side A's kernel answers with RST+ACK.
func TestIntegration_TCPRSTFloodMetersTrackedResets(t *testing.T) {
	m := newIntegrationManager(t)
	_ = newRouter(t)

	// How many of n dials to a closed port were refused, rather than timing out
	// because the reset never arrived. Half a second each: under the kernel's
	// one-second SYN retransmit, so a dropped reset is not rescued by a second
	// SYN.
	refused := func(n int) int {
		got := 0
		for i := 0; i < n; i++ {
			c, err := net.DialTimeout("tcp", "10.77.1.2:9", 500*time.Millisecond)
			if c != nil {
				_ = c.Close()
			}
			if errors.Is(err, syscall.ECONNREFUSED) {
				got++
			}
		}
		return got
	}

	applyRules(t, m, shared.Rules{}, shared.FirewallOptions{})
	if got := refused(5); got != 5 {
		t.Fatalf("control: %d of 5 dials to a closed port were refused with the module off; "+
			"the harness is not routing, so nothing below can be trusted", got)
	}

	applyRules(t, m, shared.Rules{}, shared.FirewallOptions{TCPRSTFlood: true, TCPRSTFloodLimit: 1})
	if got := refused(5); got > 3 {
		t.Errorf("%d of 5 resets reached the socket under a limit of 1/s: the meter "+
			"did not see them\ninput chain:\n  %s", got, strings.Join(chainText(t, "input"), "\n  "))
	}
}

// The order the two tests above depend on, read off the kernel's copy of the
// chain: both meters before the established accept, and so before the ICMP
// accepts that follow it. A red run here names the move that broke them.
func TestIntegration_TheMetersPrecedeTheEstablishedAccept(t *testing.T) {
	m := newIntegrationManager(t)
	applyRules(t, m, shared.Rules{}, shared.FirewallOptions{ICMPFlood: true, TCPRSTFlood: true, InvalidPackets: true})

	input := chainText(t, "input")
	est := indexOfRule(input, "ct state established,related")
	for _, meter := range []string{"@icmpflood-v4", "@icmpflood-v6", "@tcprst-v4", "@tcprst-v6"} {
		at := indexOfRule(input, meter)
		if at < 0 || est < 0 || at > est {
			t.Errorf("%s is rule %d, the established accept %d; the meter must come first\n  %s",
				meter, at, est, strings.Join(input, "\n  "))
		}
	}
}

// log_blacklist_connections logged single addresses only: a network went to a
// builder that took no log spec.
func TestIntegration_LogBlacklist_LabelsANetworkToo(t *testing.T) {
	m := newIntegrationManager(t)
	applyRules(t, m, shared.Rules{Blacklist: []string{"198.51.100.0/24", "2001:db8::/32"}},
		shared.FirewallOptions{LogBlacklist: true})

	input := chainText(t, "input")
	for _, entry := range []string{"198.51.100.0/24", "2001:db8::/32"} {
		logAt := indexOfRule(input, entry, `log prefix "`+logPrefixBlacklist+`"`)
		dropAt := indexOfRule(input, entry, "drop")
		if logAt < 0 || dropAt < 0 || logAt > dropAt {
			t.Errorf("%s: log rule %d, drop %d; a network entry must be logged before it is dropped\n  %s",
				entry, logAt, dropAt, strings.Join(input, "\n  "))
		}
	}
}
