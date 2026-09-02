package core

import (
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// A shutdown that lands before the window opens must still end it.
//
// Daemon.Stop cancels and then waits on d.wg, and the apply it is waiting for
// runs Start *after* several file writes and an nft subprocess. Cancel used to
// return immediately when the status was not pending, so that cancel was simply
// discarded: the goroutine reached Start, opened a window nobody could confirm,
// and blocked in Wait for up to an hour with Stop behind it. systemd's
// TimeoutStopSec fires first and SIGKILLs the daemon, leaving exactly the
// unconfirmed rules Cancel exists to prevent.
func TestAcceptance_ShutdownBeforeTheWindowOpensDoesNotOpenOne(t *testing.T) {
	a := NewAcceptance(time.Hour)

	a.CancelForShutdown() // the SIGTERM, arriving in the gap before Start

	if err := a.Start(time.Hour); err != nil {
		t.Fatalf("Start: %v", err)
	}

	done := make(chan bool, 1)
	go func() { done <- a.Wait() }()

	select {
	case accepted := <-done:
		if accepted {
			t.Fatal("Wait reported the rules accepted after a shutdown; nobody confirmed them")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait blocked on a window opened after shutdown had already been requested; " +
			"Stop waits behind this and systemd kills the daemon with the unconfirmed rules live")
	}

	if got := a.Status(); got != shared.AcceptanceRolledBack {
		t.Errorf("status after a shutdown-cancelled window = %q, want %q",
			got, shared.AcceptanceRolledBack)
	}
}
