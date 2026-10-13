package core

import (
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// Accept must not report success for a window that is already rolling back.
//
// A cancelled window stays Pending until Wait wakes and takes the lock, and
// Accept checked only Pending — so a confirmation in that gap answered true
// while Wait rolled back (516 of 1000 in a probe that accepted right after a
// cancel). The same for a window past its deadline whose timer has fired.
func TestAcceptance_AcceptAfterCancelIsRefusedAndWaitAgrees(t *testing.T) {
	a := NewAcceptance(time.Hour)
	_ = a.Start(time.Hour)
	a.Cancel()
	if a.Accept() {
		t.Error("Accept reported success for a window that was cancelled")
	}
	if a.Wait() {
		t.Error("Wait accepted a cancelled window")
	}
}

func TestAcceptance_AcceptPastTheDeadlineIsRefused(t *testing.T) {
	a := NewAcceptance(time.Millisecond)
	_ = a.Start(time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	if a.Accept() {
		t.Error("Accept reported success for a window whose deadline had passed")
	}
	if a.Wait() {
		t.Error("Wait accepted a window that expired")
	}
}

// The mirror image: once a confirmation has landed, a cancel is too late, and
// Wait reports the accept whichever channel select happened to pick.
func TestAcceptance_CancelAfterAcceptIsRefusedAndWaitAgrees(t *testing.T) {
	for i := 0; i < 200; i++ {
		a := NewAcceptance(time.Hour)
		_ = a.Start(time.Hour)
		if !a.Accept() {
			t.Fatal("Accept on an open window must land")
		}
		if a.CancelByOperator() {
			t.Fatal("a cancel after the accept reported that it ended the window")
		}
		if !a.Wait() {
			t.Fatalf("iteration %d: Wait rolled back a window Accept had confirmed", i)
		}
		if a.Reason() != ReasonTimeout {
			t.Fatalf("a refused cancel rewrote the reason to %q", a.Reason())
		}
	}
}

// A window ended by panic mode or by the core stopping says so. Both used
// plain Cancel, which recorded nothing, so the audit line read "timeout".
func TestAcceptance_EveryCancelRecordsWhoEndedTheWindow(t *testing.T) {
	cases := []struct {
		name   string
		cancel func(*Acceptance)
		want   string
	}{
		{"panic", func(a *Acceptance) { a.CancelForPanic() }, ReasonPanic},
		{"shutdown", func(a *Acceptance) { a.CancelForShutdown() }, ReasonCoreStopped},
		{"operator", func(a *Acceptance) { a.CancelByOperator() }, ReasonOperator},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAcceptance(time.Hour)
			_ = a.Start(time.Hour)
			tc.cancel(a)
			if a.Wait() {
				t.Fatal("a cancelled window was accepted")
			}
			if got := a.Reason(); got != tc.want {
				t.Errorf("Reason() = %q, want %q", got, tc.want)
			}
		})
	}
}

// The shutdown that arrives before the window opens, too: Start opens it
// already cancelled, and it has to carry the same reason.
func TestAcceptance_AWindowOpenedAfterShutdownSaysTheCoreStopped(t *testing.T) {
	a := NewAcceptance(time.Hour)
	a.CancelForShutdown()
	_ = a.Start(time.Hour)
	if a.Wait() {
		t.Fatal("a window opened during shutdown was accepted")
	}
	if got := a.Reason(); got != ReasonCoreStopped {
		t.Errorf("Reason() = %q, want %q", got, ReasonCoreStopped)
	}
}

// Firewall.CancelAcceptance is Panic's cancel; it is what has to carry the
// panic reason, not just the controller method.
func TestFirewallCancelAcceptance_RecordsPanicMode(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	_ = fw.acceptance.Start(time.Hour)
	fw.CancelAcceptance()
	if fw.acceptance.Wait() {
		t.Fatal("a window cancelled by panic mode was accepted")
	}
	st := fw.Status()
	if st.Acceptance != shared.AcceptanceRolledBack || st.AcceptanceReason != ReasonPanic {
		t.Errorf("status = %s/%q, want rolled_back/%q", st.Acceptance, st.AcceptanceReason, ReasonPanic)
	}
}
