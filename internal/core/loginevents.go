package core

import (
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

const (
	// loginEventWindow is how long a burst from one address is folded into one
	// pair of lines. A starting value, asserted by
	// TestLoginEvents_FourteenFailuresProduceExactlyTwoLines.
	loginEventWindow = 60 * time.Second

	// loginEventMaxAddrs is the ceiling on the table, because this runs in the
	// root process on input from outside and the number of distinct addresses is
	// a number a stranger chooses. Past it, events fold into one aggregate
	// bucket per event.
	loginEventMaxAddrs = 1024

	// loginEventSweep is how often closed windows are flushed. Short relative to
	// the window, so a summary lands soon after the window it describes.
	loginEventSweep = 5 * time.Second
)

// writeAuditFunc is WriteAuditLog with the path already bound, so this file can
// be tested without a filesystem.
type writeAuditFunc func(action, ruleType, detail, user string)

// debouncedEvents are the three a stranger can trigger.
//
// Successes, logout and the three enrolment events are rare and operator-caused,
// and are written immediately: an operator watching for "somebody just enrolled
// a second factor" must not wait sixty seconds to see it.
var debouncedEvents = map[shared.LoginEvent]bool{
	shared.EvLoginFailed: true,
	shared.Ev2FAFailed:   true,
	shared.EvRateLimited: true,
}

type loginEventKey struct {
	event shared.LoginEvent
	addr  string
}

type loginEventBucket struct {
	count  int
	opened time.Time
}

// loginEvents folds bursts of stranger-triggerable login events into two lines.
//
// Why this exists at all: GET_LOG hands the web process the last 200 lines, and
// a failed login is an unauthenticated operation. Roughly forty addresses at
// five attempts per ten minutes each fill those 200 visible entries within an
// hour and push an apply_rolledback out of view. The log does not fill; the page
// that shows it becomes useless.
type loginEvents struct {
	write writeAuditFunc

	mu       sync.Mutex
	buckets  map[loginEventKey]*loginEventBucket
	overflow map[shared.LoginEvent]*loginEventBucket
}

func newLoginEvents(write writeAuditFunc) *loginEvents {
	return &loginEvents{
		write:    write,
		buckets:  make(map[loginEventKey]*loginEventBucket),
		overflow: make(map[shared.LoginEvent]*loginEventBucket),
	}
}

// record takes one event. now is a parameter so the window is testable without
// sleeping through it.
func (l *loginEvents) record(p shared.LogEventPayload, now time.Time) error {
	if !shared.ValidLoginEvent(p.Event) {
		return fmt.Errorf("unknown login event: %q", p.Event)
	}

	// Parsed and normalised here rather than trusted as written: two spellings of
	// one IPv6 address would otherwise be two keys, and an unparseable value
	// would be foreign text in the record.
	addr := ""
	if a, err := netip.ParseAddr(p.Addr); err == nil {
		addr = a.String()
	}

	if !debouncedEvents[p.Event] {
		l.write(string(p.Event), "", immediateDetail(p, addr), "web")
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	key := loginEventKey{event: p.Event, addr: addr}
	if b, ok := l.buckets[key]; ok {
		if now.Sub(b.opened) < loginEventWindow {
			b.count++
			return nil
		}
		// The window closed and the sweeper has not been round yet. Flush it here
		// rather than losing the count, then open a new one.
		l.flushLocked(key, b)
		delete(l.buckets, key)
	}

	if len(l.buckets) >= loginEventMaxAddrs {
		o, ok := l.overflow[p.Event]
		if !ok {
			o = &loginEventBucket{opened: now}
			l.overflow[p.Event] = o
		}
		o.count++
		return nil
	}

	l.buckets[key] = &loginEventBucket{count: 1, opened: now}
	// The first immediately, so an operator who is watching sees it.
	l.write(string(p.Event), "", addrDetail(addr), "web")
	return nil
}

// sweep writes the summaries for every window that has closed.
func (l *loginEvents) sweep(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for key, b := range l.buckets {
		if now.Sub(b.opened) < loginEventWindow {
			continue
		}
		l.flushLocked(key, b)
		delete(l.buckets, key)
	}
	for ev, b := range l.overflow {
		if now.Sub(b.opened) < loginEventWindow {
			continue
		}
		// The count is exact; the number of distinct addresses behind it is not
		// knowable in bounded memory, which is the whole reason for the ceiling.
		// "More than N" is the true statement, and the volume is what the record
		// is for.
		l.write(string(ev), "", fmt.Sprintf("from more than %d addresses, %d within %ds",
			loginEventMaxAddrs, b.count, int(loginEventWindow/time.Second)), "web")
		delete(l.overflow, ev)
	}
}

// flushLocked writes the closing summary for one bucket. l.mu must be held.
func (l *loginEvents) flushLocked(key loginEventKey, b *loginEventBucket) {
	if b.count <= 1 {
		return // the one line written when it opened is the whole story
	}
	detail := fmt.Sprintf("%d more within %ds", b.count-1, int(loginEventWindow/time.Second))
	if key.addr != "" {
		detail = "from " + key.addr + ", " + detail
	}
	l.write(string(key.event), "", detail, "web")
}

// trackedAddrs reports how many buckets are held. For the ceiling test.
func (l *loginEvents) trackedAddrs() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// run sweeps until stop is closed.
func (l *loginEvents) run(stop <-chan struct{}) {
	ticker := time.NewTicker(loginEventSweep)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			// One last sweep on the way out, so a burst in progress at shutdown
			// still leaves its summary behind.
			l.sweep(time.Now().Add(loginEventWindow))
			return
		case t := <-ticker.C:
			l.sweep(t)
		}
	}
}

func addrDetail(addr string) string {
	if addr == "" {
		return ""
	}
	return "from " + addr
}

// immediateDetail is the detail for an event that is not debounced.
func immediateDetail(p shared.LogEventPayload, addr string) string {
	d := addrDetail(addr)
	if p.Event != shared.EvRecoveryUsed {
		return d
	}
	left := fmt.Sprintf("%d recovery codes left", p.Left)
	if d == "" {
		return left
	}
	return d + ", " + left
}
