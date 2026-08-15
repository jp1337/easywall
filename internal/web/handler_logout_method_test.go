package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Signing out changes state, so it must not be reachable by a safe method.
//
// Go's CrossOriginProtection checks the Origin and Sec-Fetch-Site headers of
// unsafe methods only — GET, HEAD and OPTIONS are exempt by design, because a
// safe method is not supposed to change anything. /logout was a GET, so it was
// outside that protection entirely. Measured against the running server before
// the change:
//
//	GET /logout   Origin: https://evil.example, Sec-Fetch-Site: cross-site  → 303
//	GET /dashboard with the same cookie                                     → 303 (signed out)
//	POST /settings Origin: https://evil.example                             → 403
//
// One <img src="https://the-host:12227/logout"> on any page the operator had
// open ended their session and revoked the cookie.
func TestSigningOutIsNotReachableWithASafeMethod(t *testing.T) {
	srv := newTestServer(t, newFakeCore(t))

	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	rec := httptest.NewRecorder()
	srv.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /logout answered %d; a safe method must not end a session "+
			"— CrossOriginProtection does not see GET, so this is reachable from any origin", rec.Code)
	}
}

// And the cross-origin POST is refused by the protection that now covers it,
// while the same-origin one goes through.
func TestSigningOutRefusesACrossOriginPost(t *testing.T) {
	srv := newTestServer(t, newFakeCore(t))

	cross := httptest.NewRequest(http.MethodPost, "/logout", nil)
	cross.Header.Set("Origin", "https://evil.example")
	cross.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	srv.router.ServeHTTP(rec, cross)
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-origin POST /logout answered %d, want 403", rec.Code)
	}

	same := httptest.NewRequest(http.MethodPost, "/logout", nil)
	same.Header.Set("Sec-Fetch-Site", "same-origin")
	rec = httptest.NewRecorder()
	srv.router.ServeHTTP(rec, same)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("same-origin POST /logout answered %d, want 303", rec.Code)
	}
}
