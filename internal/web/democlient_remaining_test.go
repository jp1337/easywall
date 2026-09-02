package web

import (
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The demo drives a real timer, so the public deployment shows a real window.
// A demo that reports 0 while its own acceptance is pending is the shape of
// false-green this repository has paid for before.
func TestDemo_StatusCarriesTheRemainingSeconds(t *testing.T) {
	c := NewDemoClient()

	if _, err := c.GetStatus(); err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if err := c.SaveRules("tcp", []shared.PortRule{{Port: "8443"}}); err != nil {
		t.Fatalf("SaveRules: %v", err)
	}
	if err := c.ApplyRules(); err != nil {
		t.Fatalf("ApplyRules: %v", err)
	}

	status, err := c.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus after apply: %v", err)
	}
	if status.Acceptance != shared.AcceptancePending {
		t.Fatalf("acceptance after apply = %q, want pending", status.Acceptance)
	}
	if status.AcceptanceRemaining != 120 {
		t.Fatalf("AcceptanceRemaining in the demo's open window = %d, want 120 "+
			"(the demo's own default duration); a demo that reports no time left is "+
			"a countdown that never runs on the public deployment",
			status.AcceptanceRemaining)
	}

	if _, err := c.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	after, err := c.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus after accept: %v", err)
	}
	if after.AcceptanceRemaining != 0 {
		t.Errorf("AcceptanceRemaining after a confirmation = %d, want 0", after.AcceptanceRemaining)
	}
}
