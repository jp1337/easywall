package web

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

// SessionIDKey holds a random per-session identifier, so a session can be
// revoked before it expires on its own.
const SessionIDKey = "sid"

// revokedSessions remembers sessions that were logged out.
//
// Sessions live in a signed cookie: the server keeps no record of them, so
// "log out" only asked the *browser* to drop the cookie. The cookie value
// itself stayed valid for the rest of its lifetime, and anyone still holding it
// — a shared machine, a copied value, a proxy log — remained signed in to a
// firewall's administration interface after the button said they were not.
//
// Entries are held for one session lifetime, after which the cookie is refused
// on its own age and there is nothing left to remember. The set therefore stays
// as small as the number of logouts in the last ten minutes.
//
// A restart clears it. That is a real gap and a narrow one: it needs a logout,
// a restart, and a reuse of the same cookie, all inside one session lifetime.
// Closing it properly means server-side sessions, which is a different design
// and is on the roadmap with multi-user support.
var revokedSessions = struct {
	mu sync.Mutex
	at map[string]time.Time
}{at: make(map[string]time.Time)}

// newSessionID returns a random identifier for a session.
func newSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Without a usable identifier the session simply cannot be revoked
		// early; it still expires on its own. Failing the login instead would
		// turn an entropy hiccup into a lockout.
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// revokeSession marks a session identifier as logged out.
func revokeSession(id string) {
	if id == "" {
		return
	}
	now := time.Now()

	revokedSessions.mu.Lock()
	defer revokedSessions.mu.Unlock()

	for sid, at := range revokedSessions.at {
		if now.Sub(at) > time.Duration(SessionLifetime)*time.Second {
			delete(revokedSessions.at, sid)
		}
	}
	revokedSessions.at[id] = now
}

// sessionRevoked reports whether this session was logged out.
func sessionRevoked(id string) bool {
	if id == "" {
		return false
	}
	revokedSessions.mu.Lock()
	defer revokedSessions.mu.Unlock()

	at, ok := revokedSessions.at[id]
	if !ok {
		return false
	}
	return time.Since(at) <= time.Duration(SessionLifetime)*time.Second
}
