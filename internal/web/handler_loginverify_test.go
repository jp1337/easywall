package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// enrol puts a known secret and a known set of recovery codes on the server and
// returns the plain codes.
func enrol(t *testing.T, s *Server) (secret string, codes []string) {
	t.Helper()
	secret = "JBSWY3DPEHPK3PXP"
	plain, hashes, err := newRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.cfg.SaveTOTP(secret, hashes); err != nil {
		t.Fatal(err)
	}
	return secret, plain
}

func currentCode(t *testing.T, secret string) string {
	t.Helper()
	raw, err := decodeTOTPSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	return totpAt(raw, stepAt(time.Now()))
}

// With a factor set, the password step ends in a redirect, not a session.
func TestLoginPOST_WithAFactorSetEndsInARedirectAndNoSession(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword("testpassword123!")
	s.cfg.Password = hash
	enrol(t, s)

	rec := doFormRequest(s, "POST", "/login", "username=admin&password=testpassword123%21")
	assertRedirect(t, rec, "/login/verify")

	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionName && c.MaxAge >= 0 && c.Value != "" {
			t.Error("the password step issued a session cookie; the second factor is skippable")
		}
	}

	// And the cookie it did issue must not authenticate anything.
	got := doRequest(s, "GET", "/dashboard", nil, rec.Result().Cookies()...)
	assertRedirect(t, got, "/login")
}

// With no factor set, nothing about the login changes.
func TestLoginPOST_WithNoFactorIsUnchanged(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword("testpassword123!")
	s.cfg.Password = hash

	rec := doFormRequest(s, "POST", "/login", "username=admin&password=testpassword123%21")
	assertRedirect(t, rec, "/dashboard")
}

// The verify page is a redirect to /login for anyone without a valid pending
// state, and costs nothing.
func TestLoginVerifyGET_WithoutAPendingStateGoesBackToLogin(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrol(t, s)

	assertRedirect(t, doRequest(s, "GET", "/login/verify", nil), "/login")
	assertRedirect(t, doFormRequest(s, "POST", "/login/verify", "code=123456"), "/login")
}

func TestLoginVerify_ACorrectCodeSignsIn(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword("testpassword123!")
	s.cfg.Password = hash
	secret, _ := enrol(t, s)

	first := doFormRequest(s, "POST", "/login", "username=admin&password=testpassword123%21")
	pending := first.Result().Cookies()

	rec := doFormRequest(s, "POST", "/login/verify", "code="+currentCode(t, secret), pending...)
	assertRedirect(t, rec, "/dashboard")

	got := doRequest(s, "GET", "/dashboard", nil, rec.Result().Cookies()...)
	assertStatus(t, got, http.StatusOK)
}

// The same code twice: the second is refused.
func TestLoginVerify_TheSameCodeTwiceIsRefused(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword("testpassword123!")
	s.cfg.Password = hash
	secret, _ := enrol(t, s)
	code := currentCode(t, secret)

	first := doFormRequest(s, "POST", "/login", "username=admin&password=testpassword123%21")
	assertRedirect(t, doFormRequest(s, "POST", "/login/verify", "code="+code, first.Result().Cookies()...), "/dashboard")

	second := doFormRequest(s, "POST", "/login", "username=admin&password=testpassword123%21")
	rec := doFormRequest(s, "POST", "/login/verify", "code="+code, second.Result().Cookies()...)
	assertRedirect(t, rec, "/login/verify")

	// A cookie named SessionName is not proof of a session: setFlash writes one
	// carrying "verify_failed" so the message survives the redirect, exactly as
	// it already does for /login's own invalid_credentials flash, and that
	// cookie has no SessionUserKey in it. What actually matters — whether it
	// authenticates anything — is answered the same way the sibling tests
	// answer it above: try an authenticated route with it.
	got := doRequest(s, "GET", "/dashboard", nil, rec.Result().Cookies()...)
	assertRedirect(t, got, "/login")
}

// A recovery code signs in, is consumed, and leaves seven.
func TestLoginVerify_ARecoveryCodeSignsInAndIsConsumed(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword("testpassword123!")
	s.cfg.Password = hash
	_, codes := enrol(t, s)

	first := doFormRequest(s, "POST", "/login", "username=admin&password=testpassword123%21")
	rec := doFormRequest(s, "POST", "/login/verify", "code="+codes[0], first.Result().Cookies()...)
	assertRedirect(t, rec, "/dashboard")

	if n := len(s.cfg.RecoveryCodes()); n != recoveryCodeCount-1 {
		t.Errorf("%d codes left, want %d", n, recoveryCodeCount-1)
	}

	// And it cannot be used again.
	second := doFormRequest(s, "POST", "/login", "username=admin&password=testpassword123%21")
	again := doFormRequest(s, "POST", "/login/verify", "code="+codes[0], second.Result().Cookies()...)
	assertRedirect(t, again, "/login/verify")
}

// Three wrong codes and the pending state dies. The message does not say which
// factor failed.
func TestLoginVerify_ThreeWrongCodesEndTheAttempt(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword("testpassword123!")
	s.cfg.Password = hash
	enrol(t, s)

	first := doFormRequest(s, "POST", "/login", "username=admin&password=testpassword123%21")
	cookies := first.Result().Cookies()

	for i := 1; i <= pendingMaxAttempts; i++ {
		rec := doFormRequest(s, "POST", "/login/verify", "code=000000", cookies...)
		want := "/login/verify"
		if i == pendingMaxAttempts {
			want = "/login"
		}
		if loc := rec.Header().Get("Location"); loc != want {
			t.Fatalf("attempt %d redirected to %q, want %q", i, loc, want)
		}
		if c := rec.Result().Cookies(); len(c) > 0 {
			cookies = c
		}
	}
}

// A password change ends half-logins too. The pending state carries the
// fingerprint for exactly this.
func TestLoginVerify_APasswordChangeInvalidatesAPendingState(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword("testpassword123!")
	s.cfg.Password = hash
	secret, _ := enrol(t, s)

	first := doFormRequest(s, "POST", "/login", "username=admin&password=testpassword123%21")
	pending := first.Result().Cookies()

	newHash, _ := HashPassword("adifferentpassword123")
	if err := s.cfg.SaveCredentials("admin", newHash); err != nil {
		t.Fatal(err)
	}

	rec := doFormRequest(s, "POST", "/login/verify", "code="+currentCode(t, secret), pending...)
	assertRedirect(t, rec, "/login")
}

// The bound the roadmap asks for, as an executable claim rather than a
// paragraph: one intermediate state allows 3 code attempts; a new intermediate
// state costs one password round; those are capped at 5 per 10 minutes per
// address. That is 15 code attempts per 10 minutes per address — and the
// sixteenth does not get through.
func TestLoginVerify_TheSixteenthCodeAttemptDoesNotGetThrough(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword("testpassword123!")
	s.cfg.Password = hash
	enrol(t, s)

	const addr = "203.0.113.200:44444"
	post := func(path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = addr // one address for the whole run, unlike doFormRequest
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		s.router.ServeHTTP(rec, req)
		return rec
	}

	attempts := 0
	for round := 1; round <= 6; round++ {
		login := post("/login", "username=admin&password=testpassword123%21")
		if login.Code == http.StatusTooManyRequests {
			if attempts != pendingMaxAttempts*5 {
				t.Fatalf("the limiter refused password round %d after %d code attempts, want %d",
					round, attempts, pendingMaxAttempts*5)
			}
			return
		}
		cookies := login.Result().Cookies()
		for i := 0; i < pendingMaxAttempts; i++ {
			rec := post("/login/verify", "code=000000", cookies...)
			attempts++
			if attempts > pendingMaxAttempts*5 {
				t.Fatalf("code attempt %d got through; the second step is a six-digit brute force", attempts)
			}
			if c := rec.Result().Cookies(); len(c) > 0 {
				cookies = c
			}
		}
	}
	t.Fatalf("six password rounds were allowed; the limiter is not bounding the second step "+
		"(%d code attempts)", attempts)
}

// The verify page is the fourth include site of the language switcher. Whoever
// is standing at the second step must be able to change language.
func TestLoginVerifyGET_CarriesTheLanguageSwitcher(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword("testpassword123!")
	s.cfg.Password = hash
	enrol(t, s)

	first := doFormRequest(s, "POST", "/login", "username=admin&password=testpassword123%21")
	req := httptest.NewRequest("GET", "/login/verify", nil)
	for _, c := range first.Result().Cookies() {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)
	body := rec.Body.String()
	if !strings.Contains(body, `action="/language"`) {
		t.Error("the verify page has no language switcher")
	}
	// type="text", not type="number": a recovery code has letters in it, and
	// type="number" is precisely the mistake ui-check.mjs documents in its
	// header for the forwarding ports.
	if strings.Contains(body, `name="code"`) && strings.Contains(body, `type="number"`) {
		t.Error(`the code field is type="number"; a recovery code has letters in it`)
	}
	if !strings.Contains(body, `autocomplete="one-time-code"`) {
		t.Error("the code field does not offer the platform's one-time-code autofill")
	}
	if strings.Contains(body, fmt.Sprintf("%d", pendingMaxAttempts)) &&
		strings.Contains(strings.ToLower(body), "attempt") {
		t.Error("the page counts down the attempts left; it must not")
	}
}
