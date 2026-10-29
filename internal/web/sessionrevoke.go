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
// Entries are held for revocationRetention, after which the cookie is refused
// on its own age and there is nothing left to remember. The set therefore stays
// as small as the number of logouts in the last ten minutes.
//
// That retention is only sound because nothing can renew a revoked cookie's
// age, and any Save of a session re-signs it with a fresh timestamp. The rule is
// held where every save goes through — sessionStore.Save below, under this
// set's lock — not at each caller. See threat-model.md, which states it as an
// invariant.
//
// A restart clears it. That is a real gap and a narrow one: it needs a logout,
// a restart, and a reuse of the same cookie, all inside one session lifetime.
// Closing it properly means server-side sessions, which is a different design
// and is on the roadmap with multi-user support.
var revokedSessions = struct {
	mu sync.Mutex
	at map[string]time.Time
}{at: make(map[string]time.Time)}

// revocationRetention is how long a revocation is remembered: one session
// lifetime and one second. securecookie stamps a cookie in whole seconds and
// accepts it while now-stamp <= SessionLifetime, so a cookie signed in the same
// second as its logout is accepted up to SessionLifetime+1s after it. Exactly
// SessionLifetime, as this was, left that second open.
const revocationRetention = time.Duration(SessionLifetime+1) * time.Second

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
		if now.Sub(at) > revocationRetention {
			delete(revokedSessions.at, sid)
		}
	}
	revokedSessions.at[id] = now
}

// sessionRevoked reports whether this session was logged out.
func sessionRevoked(id string) bool {
	revokedSessions.mu.Lock()
	defer revokedSessions.mu.Unlock()
	return revokedLocked(id)
}

// revokedLocked is sessionRevoked for a caller already holding the lock.
func revokedLocked(id string) bool {
	if id == "" {
		return false
	}
	at, ok := revokedSessions.at[id]
	return ok && time.Since(at) <= revocationRetention
}

// sessionStore is the session cookie store with one rule added: a save never
// re-signs a revoked identity. The check runs inside Save, under the revocation
// lock, so a logout is either before the save — and the save drops `user` and
// `sid` — or after it, and then the cookie's stamp is no later than the logout.
//
// It was a rule at each call site, and the call sites behind RequireAuth were
// exempt on the ground that a revoked session never reaches them. One in flight
// does: RequireAuth authenticated at the start of the request, renewingWriter
// re-signed the same session when the response began, and a logout in between
// came back as a cookie with the same `user` and `sid` and its age at zero —
// alive one lifetime past the revocation record. The password and second-factor
// re-stamps and /apply/status's renewal had the same gap.
type sessionStore struct{ *sessions.CookieStore }

// Get goes through the request's registry with this store, not the embedded
// one, so the session it returns saves through Save below.
func (s *sessionStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(s, name)
}

// New decodes with the embedded store and re-homes the session on this one.
func (s *sessionStore) New(r *http.Request, name string) (*sessions.Session, error) {
	inner, err := s.CookieStore.New(r, name)
	sess := sessions.NewSession(s, name)
	sess.Values, sess.Options, sess.IsNew = inner.Values, inner.Options, inner.IsNew
	return sess, err
}

// Save signs the session, minus a revoked identity.
func (s *sessionStore) Save(r *http.Request, w http.ResponseWriter, sess *sessions.Session) error {
	revokedSessions.mu.Lock()
	defer revokedSessions.mu.Unlock()
	if id, _ := sess.Values[SessionIDKey].(string); revokedLocked(id) {
		delete(sess.Values, SessionUserKey)
		delete(sess.Values, SessionIDKey)
	}
	return s.CookieStore.Save(r, w, sess)
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
// anonymous visitor is still a flash. sessionStore.Save now drops it too, at
// the moment of signing; this earlier strip remains so render sees no user in
// a revoked session it reads.
func (s *Server) sessionForWrite(r *http.Request) *sessions.Session {
	sess, _ := s.store.Get(r, SessionName)
	if id, _ := sess.Values[SessionIDKey].(string); sessionRevoked(id) {
		delete(sess.Values, SessionUserKey)
		delete(sess.Values, SessionIDKey)
	}
	return sess
}
