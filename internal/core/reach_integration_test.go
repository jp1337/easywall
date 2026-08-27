//go:build integration

package core

import (
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// shared.Reachable, measured against a real kernel with a real packet.
//
// The function duplicates nft.Apply's chain order by construction — there is no
// way to compute a verdict without doing so — and this is the coupling that
// keeps the duplicate honest. A rule inserted into Apply without a matching
// branch shows up here as a disagreement rather than as a wrong warning on the
// apply screen, which is the one warning that has to be believed.
//
// The address the packet comes from is 10.77.1.2, inside the namespace the
// forwarding harness already builds; it reaches this host at 10.77.1.1, so the
// input chain is what decides. That arrival is over a veth, never "lo", so
// local is false in every case here — the one row of the chain (the loopback
// accept-on-interface) this test structurally cannot exercise.
func TestIntegration_ReachableAgreesWithTheKernel(t *testing.T) {
	for _, bin := range []string{"bash", "timeout"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("skipping: %s is not installed, and this test opens a real TCP connection", bin)
		}
	}

	m := newIntegrationManager(t)
	r := newRouter(t)

	const port = 12227
	src := netip.MustParseAddr("10.77.1.2")

	ln, err := net.Listen("tcp", "10.77.1.1:"+strconv.Itoa(port))
	if err != nil {
		t.Skipf("skipping: cannot listen on 10.77.1.1:%d: %v", port, err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	open := []shared.PortRule{{Port: strconv.Itoa(port), Description: "easywall"}}

	cases := []struct {
		name  string
		rules shared.Rules
		opts  shared.FirewallOptions
		net   shared.NetworkSettings
	}{
		{name: "the port is open", rules: shared.Rules{TCP: open}},
		{name: "nothing is open", rules: shared.Rules{}},
		{name: "the source is whitelisted and no port is open",
			rules: shared.Rules{Whitelist: []string{"10.77.1.0/24"}}},
		{name: "blacklisted and whitelisted at once",
			rules: shared.Rules{
				TCP:       open,
				Blacklist: []string{"10.77.1.2"},
				Whitelist: []string{"10.77.1.2"},
			}},
		{name: "the bogon filter is on and the source is private",
			rules: shared.Rules{TCP: open}, opts: shared.FirewallOptions{Bogons: true}},
		{name: "only a custom rule opens the port",
			rules: shared.Rules{Custom: []string{
				fmt.Sprintf("tcp dport %d accept", port)}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := shared.RulesState{Current: tc.rules, Staged: tc.rules, Backup: tc.rules}
			if err := m.Apply(state, tc.opts, tc.net); err != nil {
				t.Fatalf("Apply: %v", err)
			}

			verdict, reason := shared.Reachable(tc.rules, tc.opts, tc.net, src, port, false, false)
			accepted := tcpReaches(t, r.pidA, "10.77.1.1", port)

			switch verdict {
			case shared.ReachOpen:
				if !accepted {
					t.Errorf("Reachable said open (%s) and the kernel dropped the connection\n"+
						"input chain the kernel is holding:\n  %s", reason,
						strings.Join(chainText(t, "input"), "\n  "))
				}
			case shared.ReachBlocked:
				if accepted {
					t.Errorf("Reachable said blocked (%s) and the kernel accepted the connection\n"+
						"input chain the kernel is holding:\n  %s", reason,
						strings.Join(chainText(t, "input"), "\n  "))
				}
			case shared.ReachUnknown:
				// Nothing to assert: unknown is the honest answer and the point is
				// that it is not a wrong one. Reported so a case that becomes
				// decidable is visible rather than silently under-answering.
				t.Logf("unknown (%s); the kernel %s", reason,
					map[bool]string{true: "accepted", false: "dropped"}[accepted])
			}
		})
	}
}

// tcpReaches opens a real TCP connection from inside the namespace. bash's
// /dev/tcp rather than nc, which is three incompatible programs depending on the
// distribution; timeout bounds a connection the firewall is dropping.
func tcpReaches(t *testing.T, pid, addr string, port int) bool {
	t.Helper()
	cmd := exec.Command("nsenter", "-t", pid, "-n",
		"timeout", "2", "bash", "-c",
		fmt.Sprintf("exec 3<>/dev/tcp/%s/%d", addr, port))
	start := time.Now()
	err := cmd.Run()
	t.Logf("connect to %s:%d from the namespace took %s: %v", addr, port, time.Since(start), err)
	return err == nil
}

// The SSH brute-force chain used to end in accept, and Apply adds it to the
// input chain before the blacklist. So a blacklisted address could open an SSH
// connection as long as it stayed under the rate limit — the protection module
// outranked the list whose whole job is to refuse an address.
//
// The chain now returns. Under-rate traffic falls back into the input chain and
// meets the blacklist, then the whitelist, then the port rule. Over-rate still
// drops in sshbrute-over.
//
// A dropped connection is not, by itself, evidence of anything: it is also
// what a harness that is not routing at all looks like, and what a jump whose
// match condition never fires (no conntrack, so "ct state new" never matches)
// looks like when the blacklist happens to catch the packet by some other
// route. So this also runs the same rules minus the blacklist entry first, as
// a control, and asserts the order of the two rules in the kernel's own copy
// of the chain — the actual claim the fix rests on — rather than trusting a
// blocked connection to mean what it is supposed to.
func TestIntegration_SSHBruteForceDoesNotOutrankTheBlacklist(t *testing.T) {
	for _, bin := range []string{"bash", "timeout"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("skipping: %s is not installed, and this test opens a real TCP connection", bin)
		}
	}

	m := newIntegrationManager(t)
	r := newRouter(t)

	const port = 12228
	ln, err := net.Listen("tcp", "10.77.1.1:"+strconv.Itoa(port))
	if err != nil {
		t.Skipf("skipping: cannot listen on 10.77.1.1:%d: %v", port, err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	tcpRule := shared.PortRule{Port: strconv.Itoa(port), Description: "ssh", SSH: true}
	opts := shared.FirewallOptions{SSHBruteForce: true, SSHBruteForceConnectionLimit: 5}

	// Control: the same rules, minus the blacklist entry. A dropped connection
	// below is only evidence of the blacklist outranking the module if a
	// connection can get through this harness at all when nothing blocks it.
	controlRules := shared.Rules{TCP: []shared.PortRule{tcpRule}}
	controlState := shared.RulesState{Current: controlRules, Staged: controlRules, Backup: controlRules}
	if err := m.Apply(controlState, opts, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply (control): %v", err)
	}
	if !tcpReaches(t, r.pidA, "10.77.1.1", port) {
		t.Fatalf("control failed: an SSH-marked port under the rate limit, with no blacklist entry, " +
			"refused a connection — the harness is not routing, so nothing below can be trusted")
	}

	rules := shared.Rules{
		TCP:       []shared.PortRule{tcpRule},
		Blacklist: []string{"10.77.1.2"},
	}
	state := shared.RulesState{Current: rules, Staged: rules, Backup: rules}
	if err := m.Apply(state, opts, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// The ordering claim the fix rests on, asserted directly against the
	// kernel's own copy of the chain: the sshbrute jump must still be added
	// before the blacklist drop. If someone moves the module after the
	// blacklist, this whole fix stops being necessary — and should be noticed,
	// not silently pass because both orders happen to behave the same today.
	input := inputChainText(t, m)
	sshJump := indexOfRule(input, strconv.Itoa(port), "jump sshbrute")
	blacklistDrop := indexOfRule(input, "10.77.1.2", "drop")
	if sshJump == -1 {
		t.Fatalf("no rule in the input chain jumps to sshbrute for port %d\n  %s",
			port, strings.Join(input, "\n  "))
	}
	if blacklistDrop == -1 {
		t.Fatalf("no rule in the input chain drops 10.77.1.2\n  %s", strings.Join(input, "\n  "))
	}
	if sshJump >= blacklistDrop {
		t.Fatalf("the sshbrute jump (rule %d) is not before the blacklist drop (rule %d) in the input chain\n  %s",
			sshJump, blacklistDrop, strings.Join(input, "\n  "))
	}

	if tcpReaches(t, r.pidA, "10.77.1.1", port) {
		t.Errorf("a blacklisted address reached an SSH-marked port under the rate limit\n"+
			"sshbrute chain:\n  %s\ninput chain:\n  %s",
			strings.Join(chainText(t, "sshbrute"), "\n  "),
			strings.Join(input, "\n  "))
	}

	// And the shape, so the reason a future edit breaks this is readable rather
	// than a dropped connection with no explanation.
	last := chainText(t, "sshbrute")
	if len(last) == 0 {
		t.Fatal("sshbrute chain is empty")
	}
	if got := last[len(last)-1]; !strings.Contains(got, "return") {
		t.Errorf("sshbrute ends in %q; it must return so the input chain goes on deciding", got)
	}
}
