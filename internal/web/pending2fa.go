package web

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/sessions"
)

const (
	// pendingCookieName is its own name on purpose. sessionUser() never looks at
	// this cookie, and it cannot: it is not the session cookie, so there is no
	// way for the two to disagree about whether somebody is signed in. The bug
	// that rule exists for is documented above sessionUser in middleware.go.
	pendingCookieName = "easywall_pending"

	// pendingLifetime is how long the second step stays open, in seconds. Long
	// enough to unlock a phone and read six digits; short enough that a screen
	// left unattended is not an open door. A starting value, asserted by
	// TestPending_ExpiresAfterItsLifetime, so revising it is a revision.
	pendingLifetime = 180

	// pendingMaxAttempts is how many codes one intermediate state allows. See
	// docs/_docs/security.md for the arithmetic this produces against the
	// password step's own limit.
	pendingMaxAttempts = 3

	pendingUserKey     = "u"
	pendingCredKey     = "c"
	pendingAttemptsKey = "n"
	pendingIssuedKey   = "t"
)

// pendingLogin is the state between a correct password and an accepted code.
//
// It carries the credential fingerprint rather than trusting the name alone: a
// password change ends half-logins too, which is the same rule RequireAuth
// applies to whole ones.
type pendingLogin struct {
	User     string
	CredFP   string
	Attempts int
	IssuedAt int64
}

// readPending returns the intermediate state the request carries, or ok=false
// when there is none, it is expired, or it does not verify.
//
// Nothing here consults the *current* credential fingerprint: the caller does
// that, because "the password changed underneath this half-login" and "there is
// no half-login" are different answers and the handler shows different things
// for them.
func (s *Server) readPending(r *http.Request) (pendingLogin, bool) {
	sess, err := s.pending.Get(r, pendingCookieName)
	if err != nil || sess.IsNew {
		return pendingLogin{}, false
	}

	p := pendingLogin{}
	p.User, _ = sess.Values[pendingUserKey].(string)
	p.CredFP, _ = sess.Values[pendingCredKey].(string)
	p.Attempts, _ = sess.Values[pendingAttemptsKey].(int)
	p.IssuedAt, _ = sess.Values[pendingIssuedKey].(int64)

	if p.User == "" || p.CredFP == "" || p.IssuedAt == 0 {
		return pendingLogin{}, false
	}
	// The codec enforces its own max age, and this enforces the same number
	// against the value the cookie carries. Two checks because they fail
	// differently: the codec's is against when the cookie was signed, this is
	// against when the login began, and a re-signed cookie must not extend it.
	if time.Since(time.Unix(p.IssuedAt, 0)) > time.Duration(pendingLifetime)*time.Second {
		return pendingLogin{}, false
	}
	if p.Attempts >= pendingMaxAttempts {
		return pendingLogin{}, false
	}
	return p, true
}

// writePending stores the intermediate state.
func (s *Server) writePending(w http.ResponseWriter, r *http.Request, p pendingLogin) error {
	sess, _ := s.pending.Get(r, pendingCookieName)
	sess.Values[pendingUserKey] = p.User
	sess.Values[pendingCredKey] = p.CredFP
	sess.Values[pendingAttemptsKey] = p.Attempts
	sess.Values[pendingIssuedKey] = p.IssuedAt
	return sess.Save(r, w)
}

// clearPending ends the intermediate state.
func (s *Server) clearPending(w http.ResponseWriter, r *http.Request) {
	sess, _ := s.pending.Get(r, pendingCookieName)
	sess.Options = &sessions.Options{
		Path:     "/login",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	if err := sess.Save(r, w); err != nil {
		slog.Warn("could not clear the pending login state", "error", err)
	}
}
