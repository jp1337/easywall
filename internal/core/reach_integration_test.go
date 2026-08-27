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

	rules := shared.Rules{
		TCP:       []shared.PortRule{{Port: strconv.Itoa(port), Description: "ssh", SSH: true}},
		Blacklist: []string{"10.77.1.2"},
	}
	opts := shared.FirewallOptions{SSHBruteForce: true, SSHBruteForceConnectionLimit: 5}
	state := shared.RulesState{Current: rules, Staged: rules, Backup: rules}
	if err := m.Apply(state, opts, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if tcpReaches(t, r.pidA, "10.77.1.1", port) {
		t.Errorf("a blacklisted address reached an SSH-marked port under the rate limit\n"+
			"sshbrute chain:\n  %s\ninput chain:\n  %s",
			strings.Join(chainText(t, "sshbrute"), "\n  "),
			strings.Join(chainText(t, "input"), "\n  "))
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
