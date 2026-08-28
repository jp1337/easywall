package web

import (
	"log/slog"
	"sync/atomic"

	"github.com/jp1337/easywall/internal/shared"
)

// auditEventBuffer is how many events can be waiting for the core at once.
//
// Sixty-four, because the thing that fills it is a burst of failed logins and
// the core's own debouncer folds those into two lines anyway. When it is full
// the event is discarded and a counter is logged — more honest than
// throwing unbounded goroutines at a socket that is not answering.
const auditEventBuffer = 64

// auditEvents carries login events to the core without the request waiting.
//
// The event is a side effect of a login, not a precondition of one. A handler
// drops its event here and returns; one goroutine drains the channel. When the
// core is unreachable the line goes to the journal and the login proceeds: a
// crashed core must not take the firewall's interface with it. That is 2.7's
// principle — the interface survives a core that is down — inverted.
type auditEvents struct {
	client *CoreClient
	ch     chan shared.LogEventPayload
	lost   atomic.Uint64

	// demo suppresses the recorded address. See Record.
	demo bool
}

func newAuditEvents(c *CoreClient, demo bool) *auditEvents {
	return &auditEvents{
		client: c,
		ch:     make(chan shared.LogEventPayload, auditEventBuffer),
		demo:   demo,
	}
}

// Record queues one event. It never blocks.
//
// In demo mode the address is dropped here, which is the one place both callers
// pass through — recordLoginEvent and onLoginBlocked. The public demo is a page
// anyone on the internet can open, and the addresses of everyone who has looked
// at it are not something a demonstration needs to keep. The event itself is
// still recorded: the /log page is one of the things the demo exists to show.
//
// Omitted rather than replaced with a placeholder, because the field is already
// optional at both ends — the core's addrDetail returns "" for an empty
// address, the demo's handleLogEvent builds no detail at all — so a dash would
// be one more line of code making the same statement.
func (a *auditEvents) Record(ev shared.LoginEvent, addr string, left int, proxied bool) {
	if a.demo {
		addr = ""
	}
	select {
	case a.ch <- shared.LogEventPayload{Event: ev, Addr: addr, Left: left, Proxied: proxied}:
	default:
		n := a.lost.Add(1)
		slog.Warn("the audit event buffer is full and a login event was discarded; "+
			"the core is not draining the socket", "event", string(ev), "discarded_total", n)
	}
}

// dropped reports how many events were discarded for a full buffer.
func (a *auditEvents) dropped() uint64 { return a.lost.Load() }

// run drains the queue until stop is closed. One sender, so the socket is never
// contended and the order events were recorded in is the order they land in.
func (a *auditEvents) run(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case p := <-a.ch:
			if err := a.client.LogEvent(p.Event, p.Addr, p.Left, p.Proxied); err != nil {
				// The journal is the fallback record, and it carries everything
				// the audit entry would have: an operator diagnosing a login can
				// still find it, and the login itself already succeeded.
				slog.Warn("could not record a login event with the core; it is in this journal instead",
					"event", string(p.Event), "addr", p.Addr, "error", err)
			}
		}
	}
}
