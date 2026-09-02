package core

import (
	"testing"
	"time"
)

func TestFirewallStatus_AcceptanceRemainingIsZeroWhenNoWindowIsOpen(t *testing.T) {
	fw := newTestFirewall(t, newTestConfig(t))

	if got := fw.Status().AcceptanceRemaining; got != 0 {
		t.Errorf("AcceptanceRemaining with no window open = %d, want 0", got)
	}
}

// Rounded up, so the first render of a 120-second window says 02:00 rather than
// 01:59. The count is the only number on that screen and starting it one second
// in is wrong about it.
func TestFirewallStatus_AcceptanceRemainingCountsTheOpenWindow(t *testing.T) {
	fw := newTestFirewall(t, newTestConfig(t))

	if err := fw.acceptance.Start(120 * time.Second); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(fw.acceptance.Reset)

	got := fw.Status().AcceptanceRemaining
	if got != 120 {
		t.Fatalf("AcceptanceRemaining just after a 120s window opened = %d, want 120", got)
	}
}

func TestFirewallStatus_AcceptanceRemainingIsZeroAfterTheWindowCloses(t *testing.T) {
	fw := newTestFirewall(t, newTestConfig(t))

	if err := fw.acceptance.Start(time.Second); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fw.acceptance.Reset()

	if got := fw.Status().AcceptanceRemaining; got != 0 {
		t.Errorf("AcceptanceRemaining after Reset = %d, want 0", got)
	}
}
