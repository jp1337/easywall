package web

import (
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The demo records that somebody signed in, and not from where.
//
// The field is omitted rather than filled with a placeholder: it is already
// optional — the core's addrDetail returns "" for an empty address and the
// demo's handleLogEvent builds no detail at all — so a dash would be one more
// line of code making the same statement.
func TestDemoModeRecordsNoLoginAddress(t *testing.T) {
	a := newAuditEvents(NewDemoClient(), true)
	a.Record(shared.EvLoginOK, "203.0.113.7", 0, false)

	p := <-a.ch
	if p.Addr != "" {
		t.Errorf("the demo queued the address %q; it must record none", p.Addr)
	}
	if p.Event != shared.EvLoginOK {
		t.Errorf("event = %q; the event itself is still recorded", p.Event)
	}
}

// And a real installation still records it. The suppression is demo mode's
// alone; a firewall's audit log without addresses is not an audit log.
func TestAnOrdinaryInstallationStillRecordsTheAddress(t *testing.T) {
	a := newAuditEvents(NewDemoClient(), false)
	a.Record(shared.EvLoginOK, "203.0.113.7", 0, false)

	if p := <-a.ch; p.Addr != "203.0.113.7" {
		t.Errorf("address = %q, want 203.0.113.7", p.Addr)
	}
}
