package web

import (
	"sync"
	"time"
)

// pendingAttempts counts failed second-factor attempts per intermediate state,
// on the server, keyed by the random id the pending cookie carries.
//
// The count used to live in the easywall_pending cookie and nowhere else, so
// the budget only bound a browser that kept the cookie the server handed back.
// An attacker keeps the first one: one correct password round, then the same
// frozen cookie replayed against POST /login/verify — measured at 200 guesses
// out of one password round, 52,836/s in process, which is ~9.5M inside one
// 180-second window against the three codes out of a million that
// totpWindowLogin accepts at any instant. pendingMaxAttempts was a suggestion,
// and the tests that were supposed to hold it all did
// `if c := rec.Result().Cookies(); len(c) > 0 { cookies = c }` — they modelled a
// cooperating browser. TestPending_AFrozenCookieDoesNotBuyMoreAttempts is the
// one that does not.
//
// newPendingStore's own comment objects to a server-side table — "a map any
// stranger can fill is memory any stranger occupies" — and that objection is
// answered rather than overruled. An entry is created only by
// recordPendingAttempt, which is reachable only with a pending cookie this
// server signed, which is issued only behind a verified password, which
// LoginRateLimit caps at 5 per 10 minutes per address. A stranger with no
// password fills nothing; the operator's own worst case is 5 entries of a few
// dozen bytes per address per ten minutes. Entries are swept at pendingLifetime
// on every write, so the map is as large as the failed logins of the last three
// minutes and no larger.
//
// A restart empties it, and an id nothing is known about therefore starts with
// a fresh budget rather than being refused. That direction is deliberate and is
// the same weighing totpReplay makes: the cost of forgetting is at most one
// extra round of three attempts behind a password that is itself limited, and
// the cost of the other direction would be a restart that locks an operator out
// of their own firewall.
var pendingAttempts = struct {
	mu sync.Mutex
	at map[string]pendingCounter
}{at: make(map[string]pendingCounter)}

type pendingCounter struct {
	n     int
	begun time.Time
}

// pendingAttemptCount returns how many attempts this intermediate state has
// spent. An id with no entry has spent none — see the note on restarts above.
func pendingAttemptCount(id string) int {
	if id == "" {
		return 0
	}
	pendingAttempts.mu.Lock()
	defer pendingAttempts.mu.Unlock()

	c, ok := pendingAttempts.at[id]
	if !ok || time.Since(c.begun) > time.Duration(pendingLifetime)*time.Second {
		return 0
	}
	return c.n
}

// recordPendingAttempt charges one failed attempt to this intermediate state and
// returns the new total.
func recordPendingAttempt(id string) int {
	if id == "" {
		return 0
	}
	now := time.Now()

	pendingAttempts.mu.Lock()
	defer pendingAttempts.mu.Unlock()

	for other, c := range pendingAttempts.at {
		if now.Sub(c.begun) > time.Duration(pendingLifetime)*time.Second {
			delete(pendingAttempts.at, other)
		}
	}

	c, ok := pendingAttempts.at[id]
	if !ok {
		c = pendingCounter{begun: now}
	}
	c.n++
	pendingAttempts.at[id] = c
	return c.n
}
