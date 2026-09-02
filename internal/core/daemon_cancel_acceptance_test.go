package core

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// Cancel has existed since 2.7 and was reachable only from shutdown. The
// operator's way to it is this command, and the answer it returns is the whole
// point: a rollback arriving after the window closed changed nothing, and
// reporting success for it is the same lie "accepted and applied successfully"
// used to tell after a timeout.
func TestAcceptance_CancelReportsWhetherThereWasAWindow(t *testing.T) {
	a := NewAcceptance(time.Minute)

	if a.Cancel() {
		t.Error("Cancel on an idle controller reported that it cancelled something")
	}

	if err := a.Start(time.Minute); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !a.Cancel() {
		t.Error("Cancel on an open window reported that there was nothing to cancel")
	}
	// A second one is a no-op and says so, rather than closing a closed channel.
	if a.Cancel() {
		t.Error("a second Cancel reported that it cancelled a second window")
	}
}

// A CancelByOperator that lands with no open window must not touch the
// reason. Wait's timeout branch sets the status to RolledBack and then logs
// before returning; an operator cancel arriving in that gap has nothing left
// to cancel, and if it wrote the reason anyway it would relabel a genuine
// timeout as an operator rollback — the exact mislabel this command exists to
// prevent. Simulated here by cancelling against a controller that was never
// started, i.e. status stays AcceptanceIdle throughout, which is the same
// "not Pending" shape as landing after Wait's timeout branch has already run.
func TestAcceptance_CancelByOperatorDoesNotStompReasonWithNoWindowOpen(t *testing.T) {
	a := NewAcceptance(time.Minute)
	before := a.Reason()

	if a.CancelByOperator() {
		t.Fatal("CancelByOperator reported success against a controller with no open window")
	}
	if got := a.Reason(); got != before {
		t.Errorf("Reason changed from %q to %q even though nothing was cancelled", before, got)
	}
}

// The reason is what apply writes as the audit detail, so a window nobody
// touched has to read "timeout" and one the operator ended has to read
// something else — or the log records the two as the same event.
func TestAcceptance_ReasonDistinguishesATimeoutFromAnOperatorRollback(t *testing.T) {
	a := NewAcceptance(time.Minute)
	if err := a.Start(time.Minute); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := a.Reason(); got != "timeout" {
		t.Errorf("a fresh window's reason is %q, want %q", got, "timeout")
	}

	if !a.CancelByOperator() {
		t.Fatal("CancelByOperator found no open window")
	}
	if got := a.Reason(); got != "cancelled by operator" {
		t.Errorf("after an operator rollback the reason is %q, want %q",
			got, "cancelled by operator")
	}

	// The next window starts its own reason rather than inheriting the last one.
	a.Reset()
	if err := a.Start(time.Minute); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if got := a.Reason(); got != "timeout" {
		t.Errorf("a new window inherited the previous reason %q; the next apply that "+
			"simply expires would be logged as an operator rollback", got)
	}
}

func TestFirewallRollback_CarriesTheOperatorsReason(t *testing.T) {
	fw := newTestFirewall(t, newTestConfig(t))

	if fw.Rollback() {
		t.Error("Rollback reported success with no window open")
	}

	if err := fw.acceptance.Start(time.Minute); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(fw.acceptance.Reset)

	if !fw.Rollback() {
		t.Fatal("Rollback reported no open window while one was pending")
	}
	if got := fw.acceptance.Reason(); got != "cancelled by operator" {
		t.Errorf("the reason apply will write is %q, want %q", got, "cancelled by operator")
	}
}

func TestDaemonDispatch_CancelAcceptance(t *testing.T) {
	cfg := newTestConfig(t)
	d := &Daemon{cfg: cfg, firewall: newTestFirewall(t, cfg), quit: make(chan struct{})}

	// Nothing open: refused, and the refusal is the answer, not an error.
	resp := d.dispatch(shared.Command{Type: shared.CmdCancelAcceptance})
	if !resp.Success {
		t.Fatalf("CANCEL_ACCEPTANCE with no window returned an error: %s", resp.Error)
	}
	var result shared.CancelResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Cancelled {
		t.Error("CANCEL_ACCEPTANCE reported a rollback with no window open")
	}

	// A window open: cancelled.
	if err := d.firewall.acceptance.Start(time.Minute); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(d.firewall.acceptance.Reset)

	resp = d.dispatch(shared.Command{Type: shared.CmdCancelAcceptance})
	if !resp.Success {
		t.Fatalf("CANCEL_ACCEPTANCE with a window open returned an error: %s", resp.Error)
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.Cancelled {
		t.Error("CANCEL_ACCEPTANCE reported nothing to cancel while a window was open")
	}
}
