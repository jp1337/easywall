package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestARequestInFlightAtLogoutDoesNotReSignTheSession: a page that was still
// rendering when its own session was logged out.
//
// RequireAuth authenticated the session when the request began and
// renewingWriter re-signed that same session when the response started — with
// nothing between them asking again. A logout in that gap revoked the id, and
// the response that followed handed back a cookie carrying the same user and
// sid with its age reset. The revocation record is kept one SessionLifetime from
// the logout; the re-signed cookie lived one SessionLifetime from the response,
// so once the record was swept it was a working session again.
func TestARequestInFlightAtLogoutDoesNotReSignTheSession(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))
	enrollFactor(t, s)
	cookie := makeAuthCookie(t, s)

	entered, release := make(chan struct{}), make(chan struct{})
	slow := RequireAuth(s.store, s.currentCredential())(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			close(entered)
			<-release
			w.WriteHeader(http.StatusOK)
		}))

	req := httptest.NewRequest("GET", "/slow", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { slow.ServeHTTP(rec, req); close(done) }()

	<-entered
	doRequest(s, "POST", "/logout", nil, cookie)
	close(release)
	<-done

	if sessionCookie(rec.Result()) == "" {
		return // nothing re-signed at all, which is also fine
	}
	refreshed := savedSession(t, rec)
	expireTheRevocationRecords()
	if got := doRequest(s, "GET", "/dashboard", nil, refreshed); got.Code != http.StatusSeeOther {
		t.Errorf("a session logged out while one of its requests was in flight came back: "+
			"GET /dashboard with the cookie that request re-signed answered %d, want 303", got.Code)
	}
}

// TestTheRevocationRecordOutlivesTheCookieItRevokes: securecookie stamps a
// cookie in whole seconds and accepts it while now-stamp <= SessionLifetime, so
// a cookie signed in the same second as its logout is accepted up to one second
// past SessionLifetime after it. A record kept exactly SessionLifetime left that
// second open.
func TestTheRevocationRecordOutlivesTheCookieItRevokes(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))
	enrollFactor(t, s)
	cookie := makeAuthCookie(t, s)
	doRequest(s, "POST", "/logout", nil, cookie)

	revokedSessions.mu.Lock()
	for sid := range revokedSessions.at {
		revokedSessions.at[sid] = time.Now().Add(-(time.Duration(SessionLifetime)*time.Second + 500*time.Millisecond))
	}
	revokedSessions.mu.Unlock()

	// As old as the codec still accepts.
	aged := backdatedCookie(t, cookie, "test-session-key-32bytes-padding!", time.Duration(SessionLifetime)*time.Second)
	if got := doRequest(s, "GET", "/dashboard", nil, aged); got.Code != http.StatusSeeOther {
		t.Errorf("a logged-out cookie the codec still accepts outlived its revocation record: "+
			"GET /dashboard answered %d, want 303", got.Code)
	}
}
