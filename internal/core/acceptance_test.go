package core

import (
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

func TestAcceptance_InitialStatus(t *testing.T) {
	a := NewAcceptance(5 * time.Second)
	if a.Status() != shared.AcceptanceIdle {
		t.Errorf("expected idle, got %s", a.Status())
	}
}

func TestAcceptance_StartSetsPending(t *testing.T) {
	a := NewAcceptance(5 * time.Second)
	if err := a.Start(0); err != nil {
		t.Fatal(err)
	}
	if a.Status() != shared.AcceptancePending {
		t.Errorf("expected pending after Start, got %s", a.Status())
	}
}

func TestAcceptance_StartIdempotent(t *testing.T) {
	a := NewAcceptance(5 * time.Second)
	_ = a.Start(0)
	if err := a.Start(0); err != nil {
		t.Error("second Start should be idempotent")
	}
	if a.Status() != shared.AcceptancePending {
		t.Errorf("expected pending, got %s", a.Status())
	}
}

func TestAcceptance_AcceptBeforeStartIsNoop(t *testing.T) {
	a := NewAcceptance(5 * time.Second)
	a.Accept() // should not panic
	if a.Status() != shared.AcceptanceIdle {
		t.Errorf("Accept on idle should not change status, got %s", a.Status())
	}
}

func TestAcceptance_WaitAccepted(t *testing.T) {
	a := NewAcceptance(2 * time.Second)
	_ = a.Start(0)

	go func() {
		time.Sleep(50 * time.Millisecond)
		a.Accept()
	}()

	result := a.Wait()
	if !result {
		t.Error("expected Wait to return true after Accept")
	}
	if a.Status() != shared.AcceptanceAccepted {
		t.Errorf("expected accepted status, got %s", a.Status())
	}
}

func TestAcceptance_WaitTimeout(t *testing.T) {
	a := NewAcceptance(100 * time.Millisecond)
	_ = a.Start(0)

	result := a.Wait()
	if result {
		t.Error("expected Wait to return false on timeout")
	}
	if a.Status() != shared.AcceptanceRolledBack {
		t.Errorf("expected rolled_back status, got %s", a.Status())
	}
}

func TestAcceptance_Reset(t *testing.T) {
	a := NewAcceptance(100 * time.Millisecond)
	_ = a.Start(0)
	a.Wait() // let it time out
	a.Reset()
	if a.Status() != shared.AcceptanceIdle {
		t.Errorf("expected idle after Reset, got %s", a.Status())
	}
}

func TestAcceptance_AcceptMultipleTimes(t *testing.T) {
	a := NewAcceptance(2 * time.Second)
	_ = a.Start(0)

	go func() {
		time.Sleep(50 * time.Millisecond)
		a.Accept()
		a.Accept() // second Accept should be a no-op (buffered channel)
		a.Accept()
	}()

	result := a.Wait()
	if !result {
		t.Error("expected Wait to return true")
	}
}

func TestAcceptance_FullCycle(t *testing.T) {
	a := NewAcceptance(2 * time.Second)

	if a.Status() != shared.AcceptanceIdle {
		t.Fatal("expected idle initially")
	}
	_ = a.Start(0)
	if a.Status() != shared.AcceptancePending {
		t.Fatal("expected pending after Start")
	}
	go func() {
		time.Sleep(30 * time.Millisecond)
		a.Accept()
	}()
	if !a.Wait() {
		t.Fatal("expected accepted")
	}
	if a.Status() != shared.AcceptanceAccepted {
		t.Fatalf("expected accepted, got %s", a.Status())
	}
	a.Reset()
	if a.Status() != shared.AcceptanceIdle {
		t.Fatalf("expected idle after reset, got %s", a.Status())
	}
}

// The window's length has to come from the configuration in force when the
// apply starts. It used to be captured once, when the daemon was built, so
// changing it on the system settings page wrote the new value to easywall.toml
// and every later apply still used the old one — while the documentation said
// the next apply would pick it up.
func TestAcceptance_StartUsesTheDurationGivenToIt(t *testing.T) {
	a := NewAcceptance(5 * time.Minute)

	if err := a.Start(30 * time.Second); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := a.Duration(); got != 30*time.Second {
		t.Errorf("expected the window to use 30s, got %v", got)
	}

	a.Accept()
	a.Wait()
	a.Reset()

	if err := a.Start(90 * time.Second); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := a.Duration(); got != 90*time.Second {
		t.Errorf("a later apply must use the current duration, got %v", got)
	}
}

// Zero means "keep what you have", so a caller that does not know the duration
// cannot accidentally set an instant window.
func TestAcceptance_StartWithZeroKeepsThePreviousDuration(t *testing.T) {
	a := NewAcceptance(42 * time.Second)
	if err := a.Start(0); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := a.Duration(); got != 42*time.Second {
		t.Errorf("expected the constructor's duration to survive, got %v", got)
	}
}

// Shutdown during an open window. Until Stop waited for the apply goroutine and
// cancelled the window, stopping mid-window abandoned it: the unconfirmed rules
// stayed live and the rollback never ran — so a package upgrade at the wrong
// moment made a bad rule permanent.
func TestAcceptance_CancelEndsTheWaitAsNotAccepted(t *testing.T) {
	a := NewAcceptance(time.Hour) // far longer than any test may wait
	if err := a.Start(time.Hour); err != nil {
		t.Fatalf("Start: %v", err)
	}

	done := make(chan bool, 1)
	go func() { done <- a.Wait() }()

	a.Cancel()

	select {
	case accepted := <-done:
		if accepted {
			t.Error("a cancelled window must not count as accepted")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Cancel did not end the wait")
	}
	if got := a.Status(); got != shared.AcceptanceRolledBack {
		t.Errorf("expected rolled_back after cancel, got %s", got)
	}
}

// Cancel before Wait has captured the channel, and Cancel twice. Both are
// reachable from a signal handler racing an apply.
func TestAcceptance_CancelIsSafeBeforeWaitAndTwice(t *testing.T) {
	a := NewAcceptance(time.Hour)
	if err := a.Start(time.Hour); err != nil {
		t.Fatalf("Start: %v", err)
	}

	a.Cancel()
	a.Cancel() // must not panic on a second close

	done := make(chan bool, 1)
	go func() { done <- a.Wait() }()
	select {
	case accepted := <-done:
		if accepted {
			t.Error("a cancelled window must not count as accepted")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a window cancelled before Wait must not block for its full length")
	}
}

// Cancelling when nothing is pending is a no-op, not a panic.
func TestAcceptance_CancelWithNoWindowOpen(t *testing.T) {
	a := NewAcceptance(time.Minute)
	a.Cancel()
	if got := a.Status(); got != shared.AcceptanceIdle {
		t.Errorf("expected idle, got %s", got)
	}
}

// Accept has to say whether there was anything to accept. A confirmation that
// arrives after the window closed used to be discarded in silence, and the
// interface reported "Rules accepted and applied successfully" anyway — at the
// one moment when the rules had just been rolled back.
func TestAcceptance_AcceptReportsWhetherAWindowWasOpen(t *testing.T) {
	a := NewAcceptance(time.Minute)

	if a.Accept() {
		t.Error("nothing is pending, so there is nothing to accept")
	}

	if err := a.Start(time.Minute); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !a.Accept() {
		t.Error("an open window must report that the confirmation landed")
	}
	if !a.Wait() {
		t.Error("the window was accepted")
	}
	a.Reset()

	// And once it is over, a late click reports the truth again.
	if a.Accept() {
		t.Error("the window has closed; a confirmation now changes nothing")
	}
}

// The same after a rollback, which is the case an operator actually hits.
func TestAcceptance_AcceptAfterTimeoutReportsFalse(t *testing.T) {
	a := NewAcceptance(20 * time.Millisecond)
	if err := a.Start(20 * time.Millisecond); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if a.Wait() {
		t.Fatal("the window should have expired")
	}
	if a.Accept() {
		t.Error("a confirmation after the rollback must not report success")
	}
}
