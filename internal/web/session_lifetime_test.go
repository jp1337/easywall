package web

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// backdatedCookie re-stamps a session cookie with an older timestamp and signs
// it again with the same key — byte for byte what securecookie.Encode would
// have produced at that moment.
//
// It exists because the age the server enforces lives inside the signed value,
// not in the Set-Cookie header, so no amount of asking the browser proves
// anything about it. The encoding is base64url("<ts>|<value>|<mac>") over a
// SHA-256 HMAC of "<name>|<ts>|<value>".
func backdatedCookie(t *testing.T, c *http.Cookie, key string, age time.Duration) *http.Cookie {
	t.Helper()

	raw, err := base64.URLEncoding.DecodeString(c.Value)
	if err != nil {
		t.Fatalf("decode session cookie: %v", err)
	}
	parts := bytes.SplitN(raw, []byte("|"), 3)
	if len(parts) != 3 {
		t.Fatalf("session cookie has %d parts, want 3", len(parts))
	}

	ts := time.Now().UTC().Add(-age).Unix()
	mac := hmac.New(sha256.New, []byte(key))
	fmt.Fprintf(mac, "%s|%d|%s", SessionName, ts, parts[1])
	value := append([]byte(fmt.Sprintf("%d|%s|", ts, parts[1])), mac.Sum(nil)...)

	return &http.Cookie{Name: c.Name, Value: base64.URLEncoding.EncodeToString(value)}
}

// The cookie store's own idea of "expired" must be SessionLifetime, not the
// thirty days gorilla defaults to.
//
// NewCookieStore sets the codec max age from its own default and replacing
// Options afterwards leaves it alone, so this passed with a ten-minute cookie
// and a thirty-day grant. security.md states 600 seconds three times over.
func TestSessionIsRefusedOnceItIsOlderThanItsLifetime(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	const key = "test-session-key-32bytes-padding!"

	fresh := makeAuthCookie(t, s)
	if rec := doRequest(s, "GET", "/dashboard", nil, fresh); rec.Code != http.StatusOK {
		t.Fatalf("a fresh session should reach the dashboard, got %d", rec.Code)
	}

	for _, age := range []time.Duration{
		time.Duration(SessionLifetime+60) * time.Second,
		24 * time.Hour,
		29 * 24 * time.Hour,
	} {
		old := backdatedCookie(t, fresh, key, age)
		rec := doRequest(s, "GET", "/dashboard", nil, old)
		if rec.Code != http.StatusSeeOther {
			t.Errorf("a session %s old reached /dashboard with %d; it is past its %d-second lifetime "+
				"and must be sent to the login page", age, rec.Code, SessionLifetime)
		}
	}
}

// Logging out has to stay done.
//
// A revoked session is remembered for one SessionLifetime and then dropped,
// because after that the cookie is refused on its own age. That was only true
// once the store enforced the lifetime: before it did, replaying the cookie
// eleven minutes after logging out signed the visitor straight back in, with no
// restart and nothing to notice.
func TestLogoutSurvivesTheRevocationRecordExpiring(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	const key = "test-session-key-32bytes-padding!"

	cookie := makeAuthCookie(t, s)
	if rec := doRequest(s, "GET", "/dashboard", nil, cookie); rec.Code != http.StatusOK {
		t.Fatalf("setup: the session should start out valid, got %d", rec.Code)
	}

	doRequest(s, "POST", "/logout", nil, cookie)

	if rec := doRequest(s, "GET", "/dashboard", nil, cookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("the cookie still worked immediately after logout: %d", rec.Code)
	}

	// Age the revocation record past its retention, exactly as the sweep in
	// revokeSession would, and replay the same cookie aged to match.
	revokedSessions.mu.Lock()
	for sid := range revokedSessions.at {
		revokedSessions.at[sid] = time.Now().Add(-time.Duration(SessionLifetime+60) * time.Second)
	}
	revokedSessions.mu.Unlock()
	revokeSession(newSessionID()) // triggers the sweep

	replay := backdatedCookie(t, cookie, key, time.Duration(SessionLifetime+60)*time.Second)
	if rec := doRequest(s, "GET", "/dashboard", nil, replay); rec.Code != http.StatusSeeOther {
		t.Errorf("replaying a logged-out cookie %d seconds later reached /dashboard with %d; "+
			"logging out must not wear off", SessionLifetime+60, rec.Code)
	}
}
