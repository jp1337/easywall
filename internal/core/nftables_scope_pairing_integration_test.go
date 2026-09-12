//go:build integration

package core

import (
	"fmt"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// A rule's scope decides which chain or chains it belongs in, and this is the
// pairing itself, checked on both sides at once.
//
// Task 3's own test, TestForwardChain_ScopeDecidesWhatLandsHere, only ever
// looked at the forward chain — it could not have looked at the input chain,
// because it drives buildForwardChain directly through a recording adder and
// never touches Apply at all. Apply's input-chain loop called addPortAccept
// unconditionally, with no scope test of its own, so every rule reached the
// input chain regardless of what it asked for: a scope = "forwarded" rule
// meant only for a container was also opened on the host directly, invisible
// in a diff that only ever asserted the forward chain looked right. This test
// exists so that mistake cannot repeat — it asserts both sides of every
// scope, through Apply, against a real kernel, which is the only way the
// input-chain half can be reached at all (Apply resets the table and creates
// chains directly on m.conn, not through the recording seam).
func TestIntegration_PortRuleScopeDecidesTheChain(t *testing.T) {
	const port = 12250

	for _, tc := range []struct {
		name        string
		scope       shared.PortScope
		wantInput   bool
		wantForward bool
	}{
		{"empty string means host", "", true, false},
		{"host", shared.ScopeHost, true, false},
		{"forwarded", shared.ScopeForwarded, false, true},
		{"both", shared.ScopeBoth, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newIntegrationManager(t)

			rule := shared.PortRule{ID: "a1a1a1a1a1a1", Port: fmt.Sprintf("%d", port), Scope: tc.scope}
			rules := shared.Rules{TCP: []shared.PortRule{rule}}
			state := shared.RulesState{Current: rules, Staged: rules, Backup: rules}

			// docker.published_ports = "filtered" with a named network, so a
			// forwarded rule actually renders into the forward chain rather than
			// forwardPortRulesRender silently declining for lack of one — see
			// addForwardPortRules. Nothing needs to reach it; the chain's own
			// text is the assertion.
			if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{
				Docker: filteredDockerCustom("10.99.0.0/24"),
			}); err != nil {
				t.Fatalf("Apply: %v", err)
			}

			want := fmt.Sprintf("tcp dport %d", port)
			gotInput := indexOfRule(chainText(t, "input"), want, "accept") >= 0
			gotForward := indexOfRule(chainText(t, "forward"), want, "accept") >= 0

			if gotInput != tc.wantInput {
				t.Errorf("scope %q: input chain accepts port %d = %v, want %v\n--- ruleset ---\n%s",
					tc.scope, port, gotInput, tc.wantInput, ruleset(t))
			}
			if gotForward != tc.wantForward {
				t.Errorf("scope %q: forward chain accepts port %d = %v, want %v\n--- ruleset ---\n%s",
					tc.scope, port, gotForward, tc.wantForward, ruleset(t))
			}
		})
	}
}
