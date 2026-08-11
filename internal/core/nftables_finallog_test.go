//go:build integration

package core

import (
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// "Log blocked connections" must log what the policy drops, and nothing else.
//
// The log rule was written before the flush and the custom rules are appended
// by the nft CLI afterwards, so it landed in front of them: a packet a custom
// rule accepted was written to the kernel log as "easywall drop:" and then let
// through. filters.md calls this "everything the final policy drops", and an
// operator greps that prefix to find out what is being refused.
func TestIntegration_FinalDropLogSitsBehindTheCustomRules(t *testing.T) {
	m := newIntegrationManager(t)

	state := emptyState()
	state.Current.Custom = []string{"tcp dport 8443 accept"}
	opts := shared.FirewallOptions{LogBlocked: true, LogBlockedLimit: 60}

	if err := m.Apply(state, opts, shared.NetworkSettings{IPv6: shared.IPv6Config{Mode: shared.IPv6Filter}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	rules := inputChainText(t, m)
	custom := indexOfRule(rules, "dport 8443", "accept")
	logged := indexOfRule(rules, logPrefixDrop)

	if custom < 0 || logged < 0 {
		t.Fatalf("expected both a custom accept and a drop log:\n%s", strings.Join(rules, "\n"))
	}
	if logged < custom {
		t.Errorf("the drop log is at %d, in front of the custom accept at %d — "+
			"traffic the custom rule lets through is logged as dropped", logged, custom)
	}
}
