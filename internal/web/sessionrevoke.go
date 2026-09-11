package web

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/sessions"
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
// That retention is only sound because nothing can renew the cookie's age. It
// could: any Save of a session re-signs it with a fresh timestamp, and three
// paths save one they have not authenticated. sessionForWrite below is what
// holds it, and a fourth such path added without it puts this comment back to
// being wrong. See threat-model.md, which states it as an invariant.
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

// sessionForWrite returns the request's session with a revoked identity removed
// from it, for the paths that save a session without having authenticated it.
//
// Saving re-signs the cookie with a fresh timestamp, and the three public paths
// that save one — setting a flash, setting a flash with a number, and clearing
// a flash in render — kept every value the presented cookie carried. A
// logged-out cookie replayed against POST /login with a wrong password
// therefore came back re-signed: same `user`, same `sid`, age reset to zero.
// One such request every few minutes stayed inside the login rate limit, and
// once the revocation record aged out one SessionLifetime after the logout,
// the refreshed cookie was a working session again.
//
// Keeping the revocation record longer does not close it: the refresh resets
// the cookie's own age too, so there is no finite retention the same loop
// cannot outlast. Dropping the identity before the save does — a flash for an
// anonymous visitor is still a flash, and everything behind RequireAuth never
// reaches here with a revoked session in the first place.
func (s *Server) sessionForWrite(r *http.Request) *sessions.Session {
	sess, _ := s.store.Get(r, SessionName)
	if id, _ := sess.Values[SessionIDKey].(string); sessionRevoked(id) {
		delete(sess.Values, SessionUserKey)
		delete(sess.Values, SessionIDKey)
	}
	return sess
}
