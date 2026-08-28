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
}

func newAuditEvents(c *CoreClient) *auditEvents {
	return &auditEvents{client: c, ch: make(chan shared.LogEventPayload, auditEventBuffer)}
}

// Record queues one event. It never blocks.
func (a *auditEvents) Record(ev shared.LoginEvent, addr string, left int, proxied bool) {
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
