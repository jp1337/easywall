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
