package web

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
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
	enrollFactor(t, s)
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
	enrollFactor(t, s)
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

// sessionCookies returns the easywall cookies a response sets.
func sessionCookies(rec *httptest.ResponseRecorder) []*http.Cookie {
	var out []*http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionName {
			out = append(out, c)
		}
	}
	return out
}

// A session ends ten minutes after last use (privacy.md), not ten minutes after
// sign-in. Measured before: GET /dashboard, /ports, /apply/status and
// /blocked/rows with a valid session sent no Set-Cookie, so an operator who
// only browsed was sent to /login at 600 s whatever they were doing — late in
// an acceptance window of up to an hour, that turned Confirm into a rollback.
func TestASessionInUseOutlivesItsFirstTenMinutes(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	const key = "test-session-key-32bytes-padding!"

	// Signed in 500 s ago, and opens a page now.
	old := backdatedCookie(t, makeAuthCookie(t, s), key, 500*time.Second)
	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.AddCookie(old)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	renewed := sessionCookies(rec)
	if rec.Code != http.StatusOK || len(renewed) != 1 {
		t.Fatalf("GET /dashboard: %d with %d session cookies, want 200 and one renewal", rec.Code, len(renewed))
	}

	// 300 s after that page — 800 s after sign-in — the session still works.
	if rec := doRequest(s, "GET", "/dashboard", nil,
		backdatedCookie(t, renewed[0], key, 300*time.Second)); rec.Code != http.StatusOK {
		t.Errorf("a session used 300 s ago, signed in 800 s ago, got %d; it ended on sign-in age", rec.Code)
	}
}

// What a page fetches on its own is not use: an open /blocked tab polls every
// five seconds and would otherwise keep a session alive for ever. The one
// exception is the apply screen while a window is open.
func TestBackgroundRequestsRenewOnlyDuringAnOpenWindow(t *testing.T) {
	get := func(s *Server, path string, hdr map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		req.AddCookie(makeAuthCookie(t, s))
		rec := httptest.NewRecorder()
		s.router.ServeHTTP(rec, req)
		return rec
	}

	idle := newTestServerWithStatus(t, &shared.FirewallStatus{Acceptance: shared.AcceptanceIdle})
	enrollFactor(t, idle)
	if n := len(sessionCookies(get(idle, "/blocked/rows", map[string]string{"HX-Request": "true"}))); n != 0 {
		t.Errorf("an htmx poll set %d session cookies, want none", n)
	}
	if n := len(sessionCookies(get(idle, "/apply/status", map[string]string{"Sec-Fetch-Dest": "empty"}))); n != 0 {
		t.Errorf("/apply/status with no open window set %d session cookies, want none", n)
	}

	pending := newTestServerWithStatus(t, &shared.FirewallStatus{Acceptance: shared.AcceptancePending})
	enrollFactor(t, pending)
	if n := len(sessionCookies(get(pending, "/apply/status", map[string]string{"Sec-Fetch-Dest": "empty"}))); n != 1 {
		t.Errorf("/apply/status during an open window set %d session cookies, want 1", n)
	}
}

// A handler that saves the session itself — a flash — is not followed by a
// second Set-Cookie for the same name.
func TestARenewalNeverDoublesTheSessionCookie(t *testing.T) {
	s := newDemoTestServer(t)
	rec := doAuthFormRequest(t, s, "/apply/rollback", "")
	if n := len(sessionCookies(rec)); n != 1 {
		t.Errorf("POST /apply/rollback set %d session cookies, want exactly 1", n)
	}
}
