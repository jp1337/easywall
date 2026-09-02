package web

import (
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// The demo is what most visitors judge the product by, so it must answer
// AcceptanceReason the same two ways the real core does — or a demo click on
// "Roll back now" keeps telling visitors a timeout happened, on the one page
// whose whole argument is that it says what is true.
func TestDemo_StatusCarriesTheOperatorsReason(t *testing.T) {
	c := NewDemoClient()

	if err := c.ApplyRules(); err != nil {
		t.Fatalf("ApplyRules: %v", err)
	}
	if status, err := c.GetStatus(); err != nil {
		t.Fatalf("GetStatus: %v", err)
	} else if status.AcceptanceReason != "" {
		t.Errorf("AcceptanceReason while the demo's window is open = %q, want empty",
			status.AcceptanceReason)
	}

	if _, err := c.CancelAcceptance(); err != nil {
		t.Fatalf("CancelAcceptance: %v", err)
	}

	status, err := c.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus after rollback: %v", err)
	}
	if status.Acceptance != shared.AcceptanceRolledBack {
		t.Fatalf("acceptance after CancelAcceptance = %q, want rolled_back", status.Acceptance)
	}
	if status.AcceptanceReason != "cancelled by operator" {
		t.Errorf("AcceptanceReason after an operator rollback = %q, want %q",
			status.AcceptanceReason, "cancelled by operator")
	}
}

// The other way the demo's window ends — its own timer firing — has to read
// "timeout", the same token the real core's Acceptance.Reason() reports.
func TestDemo_StatusCarriesTheTimeoutReason(t *testing.T) {
	c := NewDemoClient()
	// A one-second window so the test does not wait out the demo's real
	// default; see TestDemoSend_ApplyRollback for the same pattern.
	c.demo.mu.Lock()
	c.demo.system.Acceptance = shared.AcceptanceConfig{Enabled: true, Duration: 1}
	c.demo.mu.Unlock()

	if err := c.ApplyRules(); err != nil {
		t.Fatalf("ApplyRules: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	status, err := c.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus after timer: %v", err)
	}
	if status.Acceptance != shared.AcceptanceRolledBack {
		t.Fatalf("acceptance after the timer fired = %q, want rolled_back", status.Acceptance)
	}
	if status.AcceptanceReason != "timeout" {
		t.Errorf("AcceptanceReason after the demo's timer fired = %q, want %q",
			status.AcceptanceReason, "timeout")
	}
}
