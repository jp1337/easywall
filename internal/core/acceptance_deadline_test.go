package core

import (
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// The invariant this release rests on: the number on the screen and the timer
// that fires come from one instant. Wait used to build its timer from the
// configured duration, so anything that changed the deadline without changing
// the duration would have made the screen and the rollback disagree — and the
// screen is the product's central promise being verifiable.
func TestAcceptance_WaitFiresAtTheDeadlineAndNotAtTheDuration(t *testing.T) {
	a := NewAcceptance(10 * time.Second)

	if err := a.Start(10 * time.Second); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Move the deadline in without touching duration. A Wait built from
	// Duration() sleeps ten seconds here; one built from the deadline returns
	// in under one.
	a.mu.Lock()
	a.deadline = time.Now().Add(150 * time.Millisecond)
	a.mu.Unlock()

	start := time.Now()
	if a.Wait() {
		t.Fatal("Wait reported acceptance for a window nobody confirmed")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Wait took %v; it is timing from Duration() rather than from the deadline, "+
			"so the countdown on screen and the rollback that fires are two different clocks",
			elapsed)
	}
}

// A second Start while a window is open must not extend it. The early return is
// what makes it idempotent, and a deadline written before that return would let
// a repeated APPLY_RULES hold unconfirmed rules live for as long as it keeps
// asking.
//
// Asserted on the deadline instant, not on Remaining(). Two Remaining() readings
// cannot see this: with the deadline written above the early return, the *first*
// Start captures the duration the controller was built with rather than the one
// it was called with, so the mutation corrupts the earlier reading and the later
// one looks smaller — which is what a correct implementation also produces. The
// constructor and the call are given the same duration here for the same reason:
// a mismatch between them is a second variable in a test that has to isolate one.
func TestAcceptance_SecondStartDoesNotMoveTheDeadline(t *testing.T) {
	a := NewAcceptance(2 * time.Second)

	if err := a.Start(2 * time.Second); err != nil {
		t.Fatalf("Start: %v", err)
	}
	a.mu.Lock()
	first := a.deadline
	a.mu.Unlock()

	time.Sleep(200 * time.Millisecond)
	if err := a.Start(2 * time.Second); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	a.mu.Lock()
	second := a.deadline
	a.mu.Unlock()

	if !second.Equal(first) {
		t.Fatalf("a second Start moved the deadline from %v to %v (%v later); a repeated "+
			"apply can hold unconfirmed rules live for as long as it keeps asking",
			first, second, second.Sub(first))
	}
}

func TestAcceptance_RemainingIsZeroWhenNoWindowIsOpen(t *testing.T) {
	a := NewAcceptance(time.Minute)

	if got := a.Remaining(); got != 0 {
		t.Errorf("Remaining() on an idle controller = %v, want 0", got)
	}

	if err := a.Start(time.Minute); err != nil {
		t.Fatalf("Start: %v", err)
	}
	a.Reset()
	if got := a.Remaining(); got != 0 {
		t.Errorf("Remaining() after Reset = %v, want 0", got)
	}
}

// Never negative. The web process renders this into mm:ss, and a negative
// duration renders as "-0:01" on the screen whose whole argument is that it
// says what is true.
func TestAcceptance_RemainingIsNeverNegative(t *testing.T) {
	a := NewAcceptance(time.Minute)
	if err := a.Start(time.Minute); err != nil {
		t.Fatalf("Start: %v", err)
	}

	a.mu.Lock()
	a.deadline = time.Now().Add(-30 * time.Second) // a deadline already past
	a.status = shared.AcceptancePending
	a.mu.Unlock()

	if got := a.Remaining(); got != 0 {
		t.Fatalf("Remaining() past the deadline = %v, want 0; a negative value reaches "+
			"the screen as a negative clock", got)
	}
}

// Panic must not latch. It cancels an open window too — restore.go's Panic —
// and a sticky latch there would make the first apply after `easywall-core
// resume` roll itself back instantly, with no window and no explanation.
func TestAcceptance_PlainCancelDoesNotPoisonTheNextWindow(t *testing.T) {
	a := NewAcceptance(50 * time.Millisecond)

	if err := a.Start(50 * time.Millisecond); err != nil {
		t.Fatalf("Start: %v", err)
	}
	a.Cancel()
	if a.Wait() {
		t.Fatal("a cancelled window reported acceptance")
	}
	a.Reset()

	// Through Wait, not Remaining. Remaining reports from the deadline and never
	// consults the cancel flags, so a poisoned controller still shows a full
	// count while Wait returns instantly — a Remaining-based assertion here pins
	// nothing, which the mutation proved.
	if err := a.Start(400 * time.Millisecond); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	start := time.Now()
	if a.Wait() {
		t.Fatal("the second window reported acceptance when nobody confirmed it")
	}
	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Fatalf("the window after a plain Cancel ended in %v instead of running its "+
			"400ms; a non-shutdown cancel has poisoned the controller, so the first "+
			"apply after `easywall-core resume` would roll itself back instantly", elapsed)
	}
}
