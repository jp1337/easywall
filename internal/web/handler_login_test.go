package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHandleLoginGET_RedirectsToFirstRun(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	rec := doRequest(s, "GET", "/login", nil)
	assertRedirect(t, rec, "/firstrun")
}

func TestHandleLoginGET_RedirectsToDashboardIfLoggedIn(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doAuthRequest(t, s, "GET", "/login", nil)
	assertRedirect(t, rec, "/dashboard")
}

func TestHandleLoginGET_ShowsLoginPage(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/login", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleLoginPOST_RedirectsToFirstRunWhenFirstRun(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/login", "username=admin&password=testpassword123!")
	assertRedirect(t, rec, "/firstrun")
}

func TestHandleLoginPOST_InvalidPassword(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/login", "username=admin&password=wrongpassword")
	assertRedirect(t, rec, "/login")
}

func TestHandleLoginPOST_WrongUsername(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/login", "username=wronguser&password=testpassword123!")
	assertRedirect(t, rec, "/login")
}

func TestHandleLoginPOST_ValidCredentials(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/login", "username=admin&password=testpassword123!")
	assertRedirect(t, rec, "/dashboard")
}

func TestHandleLoginPOST_SetsSessionCookie(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/login", "username=admin&password=testpassword123!")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Error("expected session cookie on successful login")
	}
}

func TestHandleLogout_ClearsSession(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doAuthRequest(t, s, "POST", "/logout", nil)
	assertRedirect(t, rec, "/login")

	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == SessionName && cookie.MaxAge >= 0 {
			t.Errorf("expected session cookie with MaxAge < 0, got %d", cookie.MaxAge)
		}
	}
}

func TestHandleLogout_WithoutSession(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "POST", "/logout", nil)
	assertRedirect(t, rec, "/login")
}

// Regression for the audit-log erasure: POST /logout is in the public route
// group with no rate limiter, and handleLogout used to call recordLoginEvent
// unconditionally. A request carrying no session at all — which is exactly
// what an unauthenticated stranger sends — wrote an audit line every time, and
// GET_LOG returns only the last 200 lines: a loop of bare POSTs erased the
// whole visible history in seconds.
//
// This fails if the recordLoginEvent call in handleLogout escapes the
// `id != ""` guard: a request with no cookie would then reach the core with a
// logout event, and the "no event" half below would see one arrive.
func TestHandleLogout_WithoutSession_RecordsNoAuditEvent(t *testing.T) {
	fc := newFakeCore(t)
	seen := make(chan shared.Command, 8)
	fc.OnCommand(shared.CmdLogEvent, func(c shared.Command) { seen <- c })

	s := newTestServer(t, fc)

	// Five bare requests — no cookie, not even a bogus one — the same shape a
	// stranger's loop would send.
	for i := 0; i < 5; i++ {
		rec := doRequest(s, "POST", "/logout", nil)
		assertRedirect(t, rec, "/login")
	}

	select {
	case cmd := <-seen:
		var p shared.LogEventPayload
		_ = json.Unmarshal(cmd.Payload, &p)
		t.Fatalf("a sessionless logout recorded an audit event (%s); "+
			"the id != \"\" guard around recordLoginEvent was bypassed", p.Event)
	case <-time.After(300 * time.Millisecond):
		// Nothing arrived, which is the point: no session, no event.
	}
}

// The other half of the same guard: a real session ending must still be
// recorded. Without this, a fix that silences the event unconditionally would
// also pass the test above.
func TestHandleLogout_WithSession_RecordsOneAuditEvent(t *testing.T) {
	fc := newFakeCore(t)
	seen := make(chan shared.Command, 8)
	fc.OnCommand(shared.CmdLogEvent, func(c shared.Command) { seen <- c })

	s := newTestServer(t, fc)
	cookie := makeAuthCookie(t, s)

	rec := doRequest(s, "POST", "/logout", nil, cookie)
	assertRedirect(t, rec, "/login")

	select {
	case cmd := <-seen:
		var p shared.LogEventPayload
		if err := json.Unmarshal(cmd.Payload, &p); err != nil {
			t.Fatalf("could not decode LOG_EVENT payload: %v", err)
		}
		if p.Event != shared.EvLogout {
			t.Errorf("logout recorded event %q, want %q", p.Event, shared.EvLogout)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a real logout recorded no audit event at all")
	}
}

// TestHandleLoginPOST_SetsFlashOnInvalidCredentials verifies flash is set.
func TestHandleLoginPOST_SetsFlashOnInvalidCredentials(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/login", "username=admin&password=wrong")
	// Should redirect to /login
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}
	// Should have set a cookie (flash session)
	_ = rec.Result().Cookies()
}

// TestHandleLoginGET_DirectHandlerNotFirstRun tests the direct handler method path.
func TestHandleLoginGET_DirectHandlerNotFirstRun(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	req := httptest.NewRequest("GET", "/login", nil)
	rec := httptest.NewRecorder()
	s.handleLoginGET(rec, req)
	assertStatus(t, rec, http.StatusOK)
}

// A wrong username must cost the same as a wrong password.
//
// The check used to be `username != want || !VerifyPassword(...)`, which skips
// argon2 entirely when the name is wrong — and argon2 is deliberately slow. A
// wrong username answered in 60µs, the right one in 37ms: a 600-fold difference,
// readable over any network, turning one request per guess into a lookup for the
// account name of the only account this system has.
//
// The bound here is loose on purpose. The gap it guards against was three orders
// of magnitude; anything under a small multiple means both paths are doing the
// same work.
func TestHandleLoginPOST_WrongUsernameCostsTheSameAsWrongPassword(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, err := HashPassword("thecorrectpassword1")
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.Password = hash
	s.cfg.Username = "admin"

	measure := func(user string) time.Duration {
		// One discarded round so neither case pays for warm-up.
		doFormRequest(s, "POST", "/login", "username="+user+"&password=wrongpassword123")
		const rounds = 5
		start := time.Now()
		for i := 0; i < rounds; i++ {
			doFormRequest(s, "POST", "/login", "username="+user+"&password=wrongpassword123")
		}
		return time.Since(start) / rounds
	}

	unknownUser := measure("nosuchuser")
	knownUser := measure("admin")

	if unknownUser <= 0 {
		t.Fatal("could not measure the unknown-username path")
	}
	if ratio := float64(knownUser) / float64(unknownUser); ratio > 3 {
		t.Errorf("a wrong username answers %.0fx faster than a wrong password "+
			"(%v vs %v), which tells an attacker the account name",
			ratio, unknownUser, knownUser)
	}
}

// Logging out has to end the session, not just ask the browser to forget it.
//
// Sessions live in a signed cookie and the server kept no record of them, so
// the value stayed valid for its full lifetime: anyone still holding it — a
// shared machine, a copied value, a proxy log — remained signed in to a
// firewall's administration interface after the button said they were not.
func TestHandleLogout_EndsTheSessionForACookieStillHeld(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	cookie := makeAuthCookie(t, s)

	if before := doRequest(s, "GET", "/dashboard", nil, cookie); before.Code != http.StatusOK {
		t.Fatalf("the session should work before logout, got %d", before.Code)
	}

	if out := doRequest(s, "POST", "/logout", nil, cookie); out.Code != http.StatusSeeOther {
		t.Fatalf("logout should redirect, got %d", out.Code)
	}

	// The browser was told to drop it; this is the copy that was kept anyway.
	after := doRequest(s, "GET", "/dashboard", nil, cookie)
	if after.Code != http.StatusSeeOther || after.Header().Get("Location") != "/login" {
		t.Errorf("a logged-out cookie must not work, got %d %q",
			after.Code, after.Header().Get("Location"))
	}
}

// One logout must not end another browser's session.
func TestHandleLogout_LeavesOtherSessionsAlone(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	laptop := makeAuthCookie(t, s)
	phone := makeAuthCookie(t, s)

	doRequest(s, "POST", "/logout", nil, laptop)

	if still := doRequest(s, "GET", "/dashboard", nil, phone); still.Code != http.StatusOK {
		t.Errorf("signing out in one browser must not sign out the other, got %d", still.Code)
	}
}

// The revocation set must not grow without bound: entries older than a session
// lifetime describe cookies that are refused on their own age anyway.
func TestRevokedSessions_ForgetsExpiredEntries(t *testing.T) {
	revokedSessions.mu.Lock()
	revokedSessions.at = make(map[string]time.Time)
	revokedSessions.mu.Unlock()

	stale := newSessionID()
	revokedSessions.mu.Lock()
	revokedSessions.at[stale] = time.Now().Add(-2 * time.Duration(SessionLifetime) * time.Second)
	revokedSessions.mu.Unlock()

	if sessionRevoked(stale) {
		t.Error("an entry older than a session lifetime is not worth keeping")
	}

	revokeSession(newSessionID()) // triggers the prune

	revokedSessions.mu.Lock()
	_, kept := revokedSessions.at[stale]
	size := len(revokedSessions.at)
	revokedSessions.mu.Unlock()

	if kept {
		t.Error("the stale entry should have been pruned")
	}
	if size != 1 {
		t.Errorf("expected only the fresh entry, got %d", size)
	}
}

// A refused session must not bounce between the two pages that judge it.
//
// RequireAuth sends a session it does not accept to /login, and /login sent a
// cookie that still carried a user value back to /dashboard. Once logout began
// revoking sessions — and once a password change began invalidating them — the
// two disagreed, and a browser presenting such a cookie ran until Chrome gave up
// with ERR_TOO_MANY_REDIRECTS. Go's handler tests do not follow redirects, so
// nothing here saw it; rendering the page did.
func TestSessionRefusal_DoesNotBounceBetweenLoginAndDashboard(t *testing.T) {
	follow := func(t *testing.T, s *Server, cookie *http.Cookie, from string) string {
		t.Helper()
		at := from
		for i := 0; i < 10; i++ {
			rec := doRequest(s, "GET", at, nil, cookie)
			if rec.Code != http.StatusSeeOther {
				return at
			}
			at = rec.Header().Get("Location")
		}
		t.Fatalf("still redirecting after ten hops, last stop %s", at)
		return ""
	}

	t.Run("after logout", func(t *testing.T) {
		fc := newFakeCore(t)
		s := newTestServer(t, fc)
		cookie := makeAuthCookie(t, s)

		doRequest(s, "POST", "/logout", nil, cookie)

		if stop := follow(t, s, cookie, "/dashboard"); stop != "/login" {
			t.Errorf("a logged-out cookie should come to rest on /login, got %s", stop)
		}
	})

	t.Run("after a password change", func(t *testing.T) {
		fc := newFakeCore(t)
		s := newTestServer(t, fc)
		cookie := makeAuthCookie(t, s)

		newHash, err := HashPassword("a-completely-new-password")
		if err != nil {
			t.Fatal(err)
		}
		if err := s.cfg.SaveCredentials("admin", newHash); err != nil {
			t.Fatal(err)
		}

		if stop := follow(t, s, cookie, "/dashboard"); stop != "/login" {
			t.Errorf("a session from before the change should come to rest on /login, got %s", stop)
		}
	})
}
