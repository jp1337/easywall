package web

import (
	"fmt"
	"sync"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

const (
	// burstThreshold matches LoginRateLimit's own ceiling, so the notification
	// fires at the moment easywall starts refusing rather than at some number
	// nobody can relate to anything.
	burstThreshold = 5
	// burstWindow is how long failures accumulate toward the threshold.
	burstWindow = 5 * time.Minute
	// burstQuiet is how long one address stays silent after firing. A phone is
	// not a log: the second hour of the same attack is not news.
	burstQuiet = 15 * time.Minute
	// burstMaxAddrs bounds the table, because the number of distinct addresses
	// is a number a stranger chooses — the same argument internal/core's
	// loginEvents makes at the same 1024, but not the same mechanism: that one
	// bounds concurrently open windows, ages them out with a five-second
	// sweep goroutine, and folds whatever does not fit into one aggregate
	// "from more than N addresses" line. This table has no goroutine (the
	// lifecycle to stop one belongs to whatever wires notification delivery
	// together, not to this file) and no aggregate fallback: a new address
	// arriving at the ceiling instead evicts every bucket that is dead — see
	// (*burstBucket).deadAt — and is refused only if the table is still full
	// once that is done.
	burstMaxAddrs = 1024
)

// countedEvents are the login events that mean somebody failed to get in.
var countedEvents = map[shared.LoginEvent]bool{
	shared.EvLoginFailed: true,
	shared.Ev2FAFailed:   true,
	shared.EvRateLimited: true,
}

type burstBucket struct {
	count   int
	opened  time.Time
	firedAt time.Time
}

// deadAt reports whether the bucket holds no state a later call for this
// address still needs: if it never fired, its accumulation window has
// closed; if it did fire, its quiet period has also closed. A bucket that is
// still counting toward the threshold, or still keeping its address quiet
// after firing, is live and must not be evicted.
func (bucket *burstBucket) deadAt(now time.Time) bool {
	if !bucket.firedAt.IsZero() {
		return now.Sub(bucket.firedAt) >= burstQuiet
	}
	return now.Sub(bucket.opened) > burstWindow
}

// loginBurst decides which failed-login activity is worth a notification.
type loginBurst struct {
	mu      sync.Mutex
	buckets map[string]*burstBucket
}

func newLoginBurst() *loginBurst {
	return &loginBurst{buckets: make(map[string]*burstBucket)}
}

// evictDeadLocked drops every bucket that no longer holds live state, only
// called once the table is at its ceiling and only ever run against the
// (bounded) table itself, not per request — so the O(n) scan happens
// exactly when hitting the ceiling requires it, not on every call. b.mu must
// be held.
func (b *loginBurst) evictDeadLocked(now time.Time) {
	for addr, bucket := range b.buckets {
		if bucket.deadAt(now) {
			delete(b.buckets, addr)
		}
	}
}

// record takes one login event and returns a Notification when this address has
// just crossed the threshold. now is a parameter so the windows are testable
// without sleeping through them.
func (b *loginBurst) record(ev shared.LoginEvent, addr string, now time.Time) *Notification {
	if !countedEvents[ev] || addr == "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	bucket, ok := b.buckets[addr]
	if !ok {
		if len(b.buckets) >= burstMaxAddrs {
			b.evictDeadLocked(now)
		}
		if len(b.buckets) >= burstMaxAddrs {
			return nil // still full after evicting; a genuinely new address is refused
		}
		bucket = &burstBucket{opened: now}
		b.buckets[addr] = bucket
	}
	if !bucket.firedAt.IsZero() {
		if now.Sub(bucket.firedAt) < burstQuiet {
			return nil // still quiet
		}
		// The quiet period is over: this failure opens a fresh window and
		// counts as its first, so an unbroken attack notifies again after
		// exactly burstThreshold more failures — the same as the first time,
		// not one more for having been quiet.
		*bucket = burstBucket{opened: now}
	}
	if now.Sub(bucket.opened) > burstWindow {
		*bucket = burstBucket{opened: now}
	}

	bucket.count++
	if bucket.count < burstThreshold {
		return nil
	}
	bucket.firedAt = now
	return &Notification{
		Event:    "failed_logins",
		Severity: "warning",
		Detail: fmt.Sprintf("%d failed sign-in attempts from %s within %d minutes",
			bucket.count, addr, int(burstWindow/time.Minute)),
		Time: now.UTC(),
	}
}
