package core

import (
	"sync"
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

// Reset must not deadlock against a concurrent Apply either — the two are
// meant to exclude each other, not to wedge each other.
func TestNftablesManager_ResetDoesNotDeadlockAgainstApply(t *testing.T) {
	m := &NftablesManager{}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = m.Apply(shared.RulesState{}, shared.FirewallOptions{}, shared.NetworkSettings{})
	}()
	go func() {
		defer wg.Done()
		_ = m.Reset()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Apply and Reset did not both return within 2s")
	}
}

// The race this mutex exists to close: Panic's Reset() and an in-flight
// apply's Apply() (or its rollback, which also calls Apply) used to share one
// *nftables.Conn with nothing serialising access to it. AddRule buffers on
// the connection and Flush drains whatever is queued — for whichever
// goroutine calls it — so unguarded concurrent access is corruption, not just
// a benign interleaving. `go test -race` is what actually proves this test is
// worth having: without the mutex it flags concurrent, unsynchronised access
// to the same *nftables.Conn fields from Apply and Reset.
func TestNftablesManager_ConcurrentApplyAndReset(t *testing.T) {
	m := &NftablesManager{}

	const n = 20
	var wg sync.WaitGroup
	wg.Add(2 * n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = m.Apply(shared.RulesState{}, shared.FirewallOptions{}, shared.NetworkSettings{})
		}()
		go func() {
			defer wg.Done()
			_ = m.Reset()
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent Apply/Reset calls did not all return within 5s")
	}
}

// Enforcing and Snapshot read m.conn too, and they are polled — Enforcing via
// Status — from a goroutine that has nothing to do with an apply in
// progress. They have to take the same lock Apply and Reset do, or the race
// detector sees exactly the read/write conflict this file is about.
func TestNftablesManager_ConcurrentApplyAndReads(t *testing.T) {
	m := &NftablesManager{}

	const n = 20
	var wg sync.WaitGroup
	wg.Add(3 * n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = m.Apply(shared.RulesState{}, shared.FirewallOptions{}, shared.NetworkSettings{})
		}()
		go func() {
			defer wg.Done()
			m.Enforcing()
		}()
		go func() {
			defer wg.Done()
			_, _ = m.Snapshot()
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent Apply/Enforcing/Snapshot calls did not all return within 5s")
	}
}
