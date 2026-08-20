package web

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

const (
	// firstRunPendingKey names the session value that carries the id. The
	// wizard already keeps state in this session — firstRunKey holds a rejected
	// submission between the POST and the re-render — so this needs no cookie
	// of its own, and adding one would mean one more MaxAge to get right. That
	// has cost this project a release before; see newSessionStore.
	firstRunPendingKey = "firstrun_pending"

	// firstRunPendingLifetime is how long a half-finished setup is held. Long
	// enough to unlock a phone, open an app, scan and type. The same value as
	// pendingSecretLifetime and a separate constant on purpose: they answer
	// different questions and may diverge.
	firstRunPendingLifetime = 10 * time.Minute
)

// pendingFirstRun is everything the wizard has collected but not yet written.
//
// Nothing in it reaches disk, and only its id travels to the browser.
// gorilla/sessions signs but does not encrypt, so a cookie value is readable
// plaintext: an argon2 digest in one is an offline cracking target handed out
// for free, and an unconfirmed secret in one is simply unnecessary.
//
// A restart mid-wizard therefore means "start again", and that costs nothing —
// the account does not exist yet either, so the first run is still the first run.
type pendingFirstRun struct {
	// The whole of the wizard's answers, not just the account: the ports and the
	// IPv6 mode are staged by applyFirstRunChoices after the write, and dropping
	// them here would silently discard everything the operator chose above the
	// password.
	Answers firstRunData

	PasswordHash string // argon2id, computed once in step 1
	Secret       string // base32, unconfirmed
	Issued       time.Time
}

// firstRunPending holds them, keyed by the id in the session.
//
// Built on the pattern of pendingSecrets in handler_2fa.go. It is bounded by
// the same argument: this is reachable only while cfg.IsFirstRun() holds, on a
// machine that has no account yet, and the wizard closes the moment one exists.
var firstRunPending = struct {
	mu sync.Mutex
	at map[string]pendingFirstRun
}{at: make(map[string]pendingFirstRun)}

// newFirstRunPendingID returns a fresh identifier for a half-finished setup.
func newFirstRunPendingID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func firstRunPendingStore(id string, p pendingFirstRun) {
	if id == "" {
		return
	}
	now := time.Now()

	firstRunPending.mu.Lock()
	defer firstRunPending.mu.Unlock()
	for k, v := range firstRunPending.at {
		if now.Sub(v.Issued) > firstRunPendingLifetime {
			delete(firstRunPending.at, k)
		}
	}
	firstRunPending.at[id] = p
}

func firstRunPendingLookup(id string) (pendingFirstRun, bool) {
	if id == "" {
		return pendingFirstRun{}, false
	}
	firstRunPending.mu.Lock()
	defer firstRunPending.mu.Unlock()

	p, ok := firstRunPending.at[id]
	if !ok || time.Since(p.Issued) > firstRunPendingLifetime {
		return pendingFirstRun{}, false
	}
	return p, true
}

func firstRunPendingClear(id string) {
	firstRunPending.mu.Lock()
	defer firstRunPending.mu.Unlock()
	delete(firstRunPending.at, id)
}
