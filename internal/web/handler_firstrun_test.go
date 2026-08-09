package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleFirstRunGET_ShowsPage(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	rec := doRequest(s, "GET", "/firstrun", nil)
	assertStatus(t, rec, http.StatusOK)
}

// The /firstrun route is only registered when IsFirstRun() is true.
// When not first run, the route doesn't exist → 404 at router level.
// We test the handler directly to cover the redirect-to-login path.
func TestHandleFirstRunGET_Direct_RedirectsWhenNotFirstRun(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	req := httptest.NewRequest("GET", "/firstrun", nil)
	rec := httptest.NewRecorder()
	s.handleFirstRunGET(rec, req)
	assertRedirect(t, rec, "/login")
}

func TestHandleFirstRunPOST_Direct_RedirectsWhenNotFirstRun(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	req := httptest.NewRequest("POST", "/firstrun", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleFirstRunPOST(rec, req)
	assertRedirect(t, rec, "/login")
}

func TestHandleFirstRunPOST_EmptyUsername(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/firstrun", "username=&password=ValidPassword123!&password_confirm=ValidPassword123!")
	assertRedirect(t, rec, "/firstrun")
}

func TestHandleFirstRunPOST_PasswordTooShort(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/firstrun", "username=admin&password=short&password_confirm=short")
	assertRedirect(t, rec, "/firstrun")
}

func TestHandleFirstRunPOST_PasswordMismatch(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/firstrun", "username=admin&password=ValidPassword123!&password_confirm=DifferentPassword!")
	assertRedirect(t, rec, "/firstrun")
}

func TestHandleFirstRunPOST_ValidSubmission(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/firstrun", "username=admin&password=ValidPassword123456!&password_confirm=ValidPassword123456!")
	assertRedirect(t, rec, "/login")
}

func TestHandleFirstRunPOST_SavesCredentials(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	if !s.cfg.IsFirstRun() {
		t.Fatal("expected first-run mode")
	}

	rec := doFormRequest(s, "POST", "/firstrun", "username=myadmin&password=MySecurePassword123!&password_confirm=MySecurePassword123!")
	assertRedirect(t, rec, "/login")

	if s.cfg.Username != "myadmin" {
		t.Errorf("expected username 'myadmin', got %q", s.cfg.Username)
	}
	if s.cfg.Password == "" {
		t.Error("expected password hash to be set")
	}
	if s.cfg.IsFirstRun() {
		t.Error("expected first-run to be complete after successful setup")
	}
}

// The stated minimum is 12 characters, so 12 has to be accepted. This test used
// to log a note wondering whether it was, and assert nothing — leaving the
// boundary of the only password rule easywall has undefined.
func TestHandleFirstRunPOST_PasswordExactly12CharsIsAccepted(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	const pw = "exactly12chr" // 12 characters
	if len(pw) != 12 {
		t.Fatalf("the test password is %d characters, not 12", len(pw))
	}

	rec := doFormRequest(s, "POST", "/firstrun",
		"username=admin&password="+pw+"&password_confirm="+pw)
	assertRedirect(t, rec, "/login")

	if s.cfg.IsFirstRun() {
		t.Error("credentials were not saved, so the account was not created")
	}
	if !VerifyPassword(pw, s.cfg.Password) {
		t.Error("the stored hash does not verify against the password that was set")
	}
}

func TestHandleFirstRunPOST_Password11Chars(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	const pw = "eleven1234!" // 11 characters
	if len(pw) != 11 {
		t.Fatalf("the test password is %d characters, not 11", len(pw))
	}

	rec := doFormRequest(s, "POST", "/firstrun",
		"username=admin&password="+pw+"&password_confirm="+pw)
	assertRedirect(t, rec, "/firstrun")

	if !s.cfg.IsFirstRun() {
		t.Error("a rejected password must not create the account")
	}
}

func TestHandleFirstRunPOST_SaveCredentialsError(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	// Make SaveCredentials fail by pointing configPath to an invalid location
	s.cfg.configPath = "/nonexistent/path/web.toml"

	rec := doFormRequest(s, "POST", "/firstrun", "username=admin&password=ValidPassword123456!&password_confirm=ValidPassword123456!")
	// Should redirect back to /firstrun (save_error flash)
	assertRedirect(t, rec, "/firstrun")
}
