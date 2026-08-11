//go:build integration

package core

import (
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The bogon filter drops RFC-1918 sources, and both the whitelist and the
// Docker bridge allowance are lists of RFC-1918 networks. The filter runs
// first, so before the exceptions existed, switching it on silently turned both
// of those features off — the packet was gone by the time either rule was
// reached. Measured on a kernel: the drop for 172.16.0.0/12 at position 17, the
// accept for 172.17.0.0/16 at 23.
func TestIntegration_BogonFilterLetsThroughWhatTheOperatorAllowed(t *testing.T) {
	m := newIntegrationManager(t)

	state := emptyState()
	state.Current.Whitelist = []string{
		"# the office",
		"192.168.1.0/24",
		"10.9.0.5",
		"2001:db8::1", // IPv6: not this module's business, must not break it
	}

	opts := shared.FirewallOptions{Bogons: true}
	docker := shared.DockerConfig{Enabled: true, CustomNetworks: []string{"172.17.0.0/16"}}

	if err := m.Apply(state, opts, shared.NetworkSettings{IPv6: shared.IPv6Config{Mode: shared.IPv6Filter}, Docker: docker}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	rules := chainText(t, "bogon")

	// Every allowed source leaves the chain before any drop.
	firstDrop := -1
	for i, r := range rules {
		if strings.Contains(r, "drop") {
			firstDrop = i
			break
		}
	}
	if firstDrop < 0 {
		t.Fatalf("the bogon chain drops nothing at all:\n%s", strings.Join(rules, "\n"))
	}

	for _, allowed := range []string{"192.168.1.0/24", "10.9.0.5"} {
		at := indexOfRule(rules, allowed, "return")
		if at < 0 {
			t.Errorf("%s is whitelisted but the bogon chain has no exception for it:\n%s",
				allowed, strings.Join(rules, "\n"))
			continue
		}
		if at > firstDrop {
			t.Errorf("the exception for %s is at %d, after the first drop at %d — it can never be reached",
				allowed, at, firstDrop)
		}
	}
	if at := indexOfRule(rules, "172.17.0.0/16", "return"); at < 0 || at > firstDrop {
		t.Errorf("the Docker bridge network has no reachable exception (found at %d, first drop at %d):\n%s",
			at, firstDrop, strings.Join(rules, "\n"))
	}

	// An exception is not an amnesty for the rest of the range: 192.168.1.0/24
	// is allowed, 192.168.0.0/16 is still dropped.
	for _, still := range []string{"0.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "240.0.0.0/4"} {
		if indexOfRule(rules, still, "drop") < 0 {
			t.Errorf("%s is no longer dropped; an exception must narrow the filter, not remove it:\n%s",
				still, strings.Join(rules, "\n"))
		}
	}

	// A comment line and an IPv6 address are not IPv4 exceptions and must not
	// produce a rule at all.
	for _, notARule := range []string{"the office", "2001:db8"} {
		if indexOfRule(rules, notARule) >= 0 {
			t.Errorf("%q became a rule in the bogon chain:\n%s", notARule, strings.Join(rules, "\n"))
		}
	}

	// The input chain reaches the module for IPv4 off loopback, and only there.
	in := inputChainText(t, m)
	jump := indexOfRule(in, "jump bogon")
	if jump < 0 {
		t.Fatalf("the input chain never jumps to the bogon chain:\n%s", strings.Join(in, "\n"))
	}
	if !strings.Contains(in[jump], `iifname != "lo"`) {
		t.Errorf("the jump to the bogon chain does not exclude loopback: %q", in[jump])
	}
}

// With no whitelist and no Docker networks the module is what it always was.
func TestIntegration_BogonFilterWithoutExceptions(t *testing.T) {
	m := newIntegrationManager(t)

	if err := m.Apply(emptyState(), shared.FirewallOptions{Bogons: true},
		shared.NetworkSettings{IPv6: shared.IPv6Config{Mode: shared.IPv6Filter}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	rules := chainText(t, "bogon")
	for _, r := range rules {
		if strings.Contains(r, "return") {
			t.Errorf("nothing was allowed, so the chain should hold no exceptions; found %q", r)
		}
	}
	if n := len(rules); n != 11 {
		t.Errorf("expected the eleven documented ranges, got %d rules:\n%s", n, strings.Join(rules, "\n"))
	}
}
