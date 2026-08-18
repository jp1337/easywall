package core

import (
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// Apply calls reset() — not Reset() — for exactly one reason: Apply already
// holds mu for the whole cycle, and a plain sync.Mutex is not reentrant. If
// Apply called Reset() instead, the second Lock from the same goroutine would
// never return. A nil connection is enough to prove this: reset() still runs,
// still checks m.conn, and still returns promptly, all under the lock Apply
// took first. Before the reset()/Reset() split, this test hung forever.
//
// This is the only unit-level test in this file. A nil-conn NftablesManager
// cannot exercise the serialisation the mutex actually provides — Apply and
// Reset both return at their own "conn is nil" check before touching conn at
// all, so a version of this file that spun up concurrent Apply/Reset/
// Enforcing/Snapshot calls against a nil connection would still pass, still
// race-clean, with `mu sync.Mutex` deleted from the struct entirely. That
// coverage needs a real *nftables.Conn and lives under the integration tag
// instead — see TestIntegration_ConcurrentApplyAndReset_TableStaysCoherent in
// nftables_mutex_integration_test.go.
func TestNftablesManager_ApplyDoesNotSelfDeadlock(t *testing.T) {
	m := &NftablesManager{} // conn is nil

	done := make(chan error, 1)
	go func() {
		done <- m.Apply(shared.RulesState{}, shared.FirewallOptions{}, shared.NetworkSettings{})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected an error for a nil connection")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Apply did not return within 2s — it deadlocked against its own lock " +
			"(Apply holds mu, and a call to Reset() rather than reset() from inside it " +
			"would try to lock mu a second time on a non-reentrant mutex)")
	}
}
