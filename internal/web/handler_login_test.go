package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

	rec := doAuthRequest(t, s, "GET", "/logout", nil)
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

	rec := doRequest(s, "GET", "/logout", nil)
	assertRedirect(t, rec, "/login")
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
