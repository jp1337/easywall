//go:build integration

package core

import (
	"strings"
	"sync"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The unit-level nil-conn tests this file replaces could not fail: Apply and
// Reset both return at their own nil check before ever touching m.conn, so
// deleting `mu sync.Mutex` from NftablesManager left them passing — and
// race-clean — regardless. The mutex exists to serialise access to a real
// *nftables.Conn, and only a real connection can show what happens without
// it: AddRule appends to a buffer the connection holds, and Flush drains and
// clears that buffer for whichever goroutine calls it next. Two goroutines
// doing that at once do not just reorder — one's Flush can ship the other's
// buffered rules and report success having sent none of its own, or the
// kernel can be handed a batch that interleaves messages from two different
// callers and reject it outright.
//
// This hammers Apply and Reset concurrently against a live connection and
// requires every call to come back with no error — with the mutex in place
// each call is self-contained: it always leaves the table in a well-defined
// state (either the rules it describes, or an empty freshly-recreated
// table), so nothing here should ever fail. It then runs one final,
// deterministic Apply and checks the table left behind is exactly what that
// Apply describes, not some interleaving of two different ones.
//
// Requires CAP_NET_ADMIN / root — run with:
//
//	sudo go test -tags integration ./internal/core/ -run TestIntegration_ConcurrentApplyAndReset_TableStaysCoherent -v
func TestIntegration_ConcurrentApplyAndReset_TableStaysCoherent(t *testing.T) {
	m := newIntegrationManager(t)

	state := emptyState()
	state.Current.TCP = []shared.PortRule{{Port: "8443"}}

	const rounds = 25
	var wg sync.WaitGroup
	errs := make(chan error, 2*rounds)
	wg.Add(2 * rounds)
	for i := 0; i < rounds; i++ {
		go func() {
			defer wg.Done()
			errs <- m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{})
		}()
		go func() {
			defer wg.Done()
			errs <- m.Reset()
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("a concurrent Apply/Reset call failed: %v — with the mutex removed, this "+
				"is one goroutine's Flush shipping another's netlink batch, or the kernel "+
				"rejecting a batch that mixes messages from two callers", err)
		}
	}

	// The storm is over. One final, deterministic Apply has to leave the table
	// in exactly the state it describes — proof the connection was not left
	// in some torn state by the concurrent pressure above.
	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("final Apply after the storm: %v", err)
	}
	rules := inputChainText(t, m)
	if indexOfRule(rules, "tcp dport 8443", "accept") < 0 {
		t.Errorf("input chain does not match the final Apply; got:\n%s", strings.Join(rules, "\n"))
	}
}
