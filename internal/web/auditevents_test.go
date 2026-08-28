package web

import (
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
			a.Record(shared.EvLoginFailed, "203.0.113.7", 0, false)
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
		a.Record(shared.EvLoginFailed, "203.0.113.7", 0, false)
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

	a.Record(shared.EvLoginOK, "203.0.113.7", 0, false)

	select {
	case cmd := <-seen:
		if cmd.Type != shared.CmdLogEvent {
			t.Errorf("the core was sent %s", cmd.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the event never reached the core")
	}
}

// run() must keep draining after a send fails. If it returned on the first
// error, an unreachable core would fill the buffer and every later event would
// be lost silently.
//
// Proved by watching the socket rather than by counting drops. A drop-counting
// version of this test is unfalsifiable at any volume below auditEventBuffer:
// with a 64-slot buffer and ten events, `dropped() == 0` holds whether run()
// drained them or stopped after the first. Here every event is answered with an
// error and every event still has to arrive, which is exactly the property.
func TestAuditEvents_AFailingCoreDoesNotStopTheDrain(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdLogEvent, errorRespFor("the core refuses everything"))

	const n = 10
	seen := make(chan struct{}, n)
	fc.OnCommand(shared.CmdLogEvent, func(shared.Command) { seen <- struct{}{} })

	a := newAuditEvents(NewCoreClient(fc.socketPath))
	stop := make(chan struct{})
	go a.run(stop)
	t.Cleanup(func() { close(stop) })

	for i := 0; i < n; i++ {
		a.Record(shared.EvLoginFailed, "203.0.113.7", 0, false)
	}
	for i := 0; i < n; i++ {
		select {
		case <-seen:
		case <-time.After(5 * time.Second):
			t.Fatalf("the core saw only %d of %d events; run() stopped draining after an error", i, n)
		}
	}
}

// A crashed core must not take the interface with it. The line goes to the
// journal and the login proceeds — 2.7's principle, inverted. There is no
// socket at all here, which is a different failure from one that answers badly.
func TestAuditEvents_AnUnreachableCoreDoesNotStopAnything(t *testing.T) {
	a := newAuditEvents(NewCoreClient("/nonexistent/easywall.sock"))
	stop := make(chan struct{})
	go a.run(stop)
	t.Cleanup(func() { close(stop) })

	// More than the buffer holds, so a run() that had stopped draining would
	// overflow and be visible in the counter.
	for i := 0; i < auditEventBuffer*2; i++ {
		a.Record(shared.EvLoginFailed, "203.0.113.7", 0, false)
	}
	time.Sleep(500 * time.Millisecond)
	if a.dropped() > auditEventBuffer {
		t.Errorf("%d of %d events were dropped against an unreachable socket; run() is not "+
			"draining at all", a.dropped(), auditEventBuffer*2)
	}
}
