//go:build integration

package core

import (
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// RuleCounters has to *report* the reserved established id, not filter it.
//
// This is the seam the health check stands on: §2's state machine reads the
// established rule's counter, and RuleCounters is where that number comes from.
// The spec said the opposite in one sentence for a while — "RuleCounters() skips
// them" — and a reader acting on it would have deleted the one number health
// looks at with every unit test in this package still green, because the map is
// only ever built from a kernel and no test under `make test` can see it.
//
// Both halves are asserted, because filtering by prefix and dropping the tag
// altogether are different mistakes with the same symptom: the reserved id is
// there, and so is an ordinary port rule's own id, from the same read of the
// same chain.
//
// The entry's existence is the claim, not its value. An idle namespace may
// legitimately have carried nothing on the established rule — a test that
// demanded a packet would be asserting that the container has traffic.
func TestIntegration_RuleCountersReportsTheReservedEstablishedID(t *testing.T) {
	m := newIntegrationManager(t)

	rule := shared.PortRule{ID: "beefcafe4321", Port: "12291", Description: "reserved id proof"}
	rules := shared.Rules{TCP: []shared.PortRule{rule}}
	state := shared.RulesState{Current: rules, Staged: rules, Backup: rules}

	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	counters, err := m.RuleCounters()
	if err != nil {
		t.Fatalf("RuleCounters: %v", err)
	}

	established, ok := counters[ReservedIDEstablished]
	if !ok {
		t.Fatalf("RuleCounters returned no entry for %q; the health check reads the "+
			"established rule's counter through this map and has nothing to read. Either "+
			"addEstablishedAccept no longer tags the rule, or RuleCounters has started "+
			"filtering reserved ids — the second is what the spec wrongly asked for, and "+
			"it fails silently everywhere else",
			ReservedIDEstablished)
	}
	// Read the field rather than asserting a value: the point is that the entry
	// came from a rule carrying an expr.Counter, and a rule with no counter
	// yields a zero-valued entry that this cannot tell apart. The unit test
	// TestEstablishedRuleIsTaggedAndCounted is what pins the counter itself.
	t.Logf("%s counted %d packets / %d bytes", ReservedIDEstablished,
		established.Packets, established.Bytes)

	if _, ok := counters[rule.ID]; !ok {
		t.Errorf("RuleCounters returned no entry for the port rule %q either, so the read "+
			"itself is broken rather than the reserved id specifically: %v", rule.ID, counters)
	}
}

// The reserved id names one rule in the table, not two.
//
// addEstablishedAccept has two callers — the input chain, and the forward chain
// from buildForwardChain — so tagging unconditionally put one id on two rules
// that count different traffic. RuleCounters filters to the input chain and
// would never have shown it; a metrics endpoint or outbound rules, both on the
// roadmap, would each have summed the two under one name.
//
// Counted over the whole table's own rendering rather than per chain, because
// the claim is about the table: whatever chain a later release adds this rule
// to, exactly one copy may carry the id.
//
// routing.mode has to be set to something that routes, or buildForwardChain
// returns before it adds anything and this test would pass on an empty forward
// chain — which is why the forward rule's presence is a t.Fatalf below rather
// than a skip. 192.0.2.0/24 needs no interface to exist.
func TestIntegration_TheReservedEstablishedIDNamesExactlyOneRule(t *testing.T) {
	m := newIntegrationManager(t)

	if err := m.Apply(emptyState(), shared.FirewallOptions{}, shared.NetworkSettings{
		Routing: shared.RoutingConfig{
			Mode:     shared.RoutingNetworks,
			Networks: []string{"192.0.2.0/24"},
		},
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	rs := ruleset(t)
	comment := `comment "` + ReservedIDEstablished + `"`
	if got := strings.Count(rs, comment); got != 1 {
		t.Errorf("%s occurs %d times in the table, want exactly 1\n"+
			"  two rules under one id sum traffic from two chains as one figure; the forward "+
			"copy buildForwardChain adds must stay untagged\n--- ruleset ---\n%s",
			comment, got, rs)
	}

	// And it is the input chain's copy that carries it — the count above would
	// also pass if the tag had simply moved to the forward chain.
	if indexOfRule(chainText(t, "input"), "ct state established,related", comment) < 0 {
		t.Errorf("the input chain has no established rule carrying %s, and that is the copy "+
			"the health check reads\n--- ruleset ---\n%s", comment, rs)
	}

	// The forward chain has the rule, with a counter and no id.
	fwd := chainText(t, "forward")
	if indexOfRule(fwd, "ct state established,related", "counter") < 0 {
		t.Fatalf("the forward chain has no established rule at all, so this test proves "+
			"nothing about the id being unique — routing.mode did not take\n--- forward ---\n%v",
			fwd)
	}
	if indexOfRule(fwd, "ct state established,related", comment) >= 0 {
		t.Errorf("the forward chain's established rule carries %s\n--- forward ---\n%v",
			comment, fwd)
	}
}
