package core

import (
	"testing"
	"time"
)

// FirewallStatus.AcceptanceReason exists so a rollback the operator clicked and
// one a timer fired are not the same sentence on the apply page. It must stay
// empty while a window is open — Acceptance.reason is set to "timeout" the
// moment Start runs, before anyone knows how the window will end, so reading
// it here for a merely Pending window would tell the operator a timeout has
// already happened.
func TestFirewallStatus_AcceptanceReasonIsEmptyWhileAWindowIsOpen(t *testing.T) {
	fw := newTestFirewall(t, newTestConfig(t))

	if err := fw.acceptance.Start(time.Minute); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(fw.acceptance.Reset)

	if got := fw.Status().AcceptanceReason; got != "" {
		t.Errorf("AcceptanceReason for an open window = %q, want empty", got)
	}
}

// AcceptanceReason is empty before any window has ever run — the zero value of
// a Firewall nobody has applied through yet.
func TestFirewallStatus_AcceptanceReasonIsEmptyWhenNoneHasRun(t *testing.T) {
	fw := newTestFirewall(t, newTestConfig(t))

	if got := fw.Status().AcceptanceReason; got != "" {
		t.Errorf("AcceptanceReason before any apply = %q, want empty", got)
	}
}

// A window nobody touches ends with reason "timeout", and Status() has to
// carry it once the window is actually over — Wait is what flips the status
// to RolledBack, so this drives it through Wait rather than asserting against
// Acceptance's internal fields directly.
func TestFirewallStatus_AcceptanceReasonReportsATimeout(t *testing.T) {
	fw := newTestFirewall(t, newTestConfig(t))

	if err := fw.acceptance.Start(10 * time.Millisecond); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(fw.acceptance.Reset)

	if accepted := fw.acceptance.Wait(); accepted {
		t.Fatal("Wait reported accepted for a window nobody confirmed")
	}

	if got := fw.Status().AcceptanceReason; got != "timeout" {
		t.Errorf("AcceptanceReason after a timeout = %q, want %q", got, "timeout")
	}
}

// The operator's own click has to read differently: Rollback (the handler
// behind /apply/rollback) calls CancelByOperator, and the goroutine blocked in
// Wait is what turns that into AcceptanceRolledBack — exactly the path a real
// apply takes, so this drives Wait on its own goroutine rather than asserting
// against Acceptance directly.
func TestFirewallStatus_AcceptanceReasonReportsAnOperatorRollback(t *testing.T) {
	fw := newTestFirewall(t, newTestConfig(t))

	if err := fw.acceptance.Start(time.Minute); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(fw.acceptance.Reset)

	done := make(chan bool, 1)
	go func() { done <- fw.acceptance.Wait() }()

	if !fw.Rollback() {
		t.Fatal("Rollback reported no open window while one was pending")
	}
	if accepted := <-done; accepted {
		t.Fatal("Wait reported accepted after an operator rollback")
	}

	if got := fw.Status().AcceptanceReason; got != "cancelled by operator" {
		t.Errorf("AcceptanceReason after an operator rollback = %q, want %q",
			got, "cancelled by operator")
	}
}
