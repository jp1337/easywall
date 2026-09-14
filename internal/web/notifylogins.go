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
	// is a number a stranger chooses. The core's loginEvents makes the same
	// argument at 1024.
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

// loginBurst decides which failed-login activity is worth a notification.
type loginBurst struct {
	mu      sync.Mutex
	buckets map[string]*burstBucket
}

func newLoginBurst() *loginBurst {
	return &loginBurst{buckets: make(map[string]*burstBucket)}
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
			return nil
		}
		bucket = &burstBucket{opened: now}
		b.buckets[addr] = bucket
	}
	if !bucket.firedAt.IsZero() {
		if now.Sub(bucket.firedAt) < burstQuiet {
			return nil // still quiet
		}
		// The quiet period just ended. This call only clears the slate: the
		// event that ends the quiet period does not itself start counting,
		// or an unbroken attack would refire on this very call whenever it
		// happens to land on the threshold-th failure (it would, every
		// time burstThreshold failures arrive back to back).
		*bucket = burstBucket{opened: now}
		return nil
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
