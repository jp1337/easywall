package web

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// The handler drops its event and moves on. A login must not wait on a socket.
func TestAuditEvents_RecordDoesNotBlockWhenNobodyIsDraining(t *testing.T) {
	fc := newFakeCore(t)
	a := newAuditEvents(NewCoreClient(fc.socketPath)) // no run() goroutine on purpose

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < auditEventBuffer*3; i++ {
			a.Record(shared.EvLoginFailed, "203.0.113.7", 0)
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Record blocked with a full buffer; a login now waits on the core")
	}
}

// When the buffer is full the event is discarded and a counter goes to the
// journal — more honest than throwing unbounded goroutines at a socket that is
// not answering.
func TestAuditEvents_AFullBufferDropsAndCounts(t *testing.T) {
	fc := newFakeCore(t)
	a := newAuditEvents(NewCoreClient(fc.socketPath))

	for i := 0; i < auditEventBuffer*2; i++ {
		a.Record(shared.EvLoginFailed, "203.0.113.7", 0)
	}
	if a.dropped() == 0 {
		t.Error("the buffer overflowed and nothing was counted; the loss is invisible")
	}
}

// The event reaches the core when there is one.
func TestAuditEvents_TheEventReachesTheCore(t *testing.T) {
	fc := newFakeCore(t)
	seen := make(chan shared.Command, 4)
	fc.OnCommand(shared.CmdLogEvent, func(c shared.Command) { seen <- c })

	a := newAuditEvents(NewCoreClient(fc.socketPath))
	stop := make(chan struct{})
	go a.run(stop)
	t.Cleanup(func() { close(stop) })

	a.Record(shared.EvLoginOK, "203.0.113.7", 0)

	select {
	case cmd := <-seen:
		if cmd.Type != shared.CmdLogEvent {
			t.Errorf("the core was sent %s", cmd.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the event never reached the core")
	}
}

// A crashed core must not take the interface with it. The line goes to the
// journal and the login proceeds — 2.7's principle, inverted.
func TestAuditEvents_AnUnreachableCoreDoesNotStopAnything(t *testing.T) {
	a := newAuditEvents(NewCoreClient("/nonexistent/easywall.sock"))
	stop := make(chan struct{})
	go a.run(stop)
	t.Cleanup(func() { close(stop) })

	for i := 0; i < 10; i++ {
		a.Record(shared.EvLoginFailed, "203.0.113.7", 0)
	}
	time.Sleep(200 * time.Millisecond)
	// Nothing to assert but the absence of a panic and of a hang: run() must
	// keep draining after a send fails, or the buffer fills and every later
	// event is lost.
	if a.dropped() != 0 {
		t.Errorf("%d event(s) were dropped against an unreachable core; run() stopped draining", a.dropped())
	}
}

// r.RemoteAddr, never X-Forwarded-For. easywall-web terminates TLS itself and is
// not assumed to sit behind a trusted proxy, so a header would let a client put
// somebody else's address in the firewall's own audit log.
func TestClientIP_IgnoresForwardingHeaders(t *testing.T) {
	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "203.0.113.7:54321"
	req.Header.Set("X-Forwarded-For", "198.51.100.1")
	req.Header.Set("X-Real-IP", "198.51.100.2")

	if got := clientIP(req); got != "203.0.113.7" {
		t.Errorf("clientIP = %q, want 203.0.113.7 — a header must never reach the audit log", got)
	}
}
