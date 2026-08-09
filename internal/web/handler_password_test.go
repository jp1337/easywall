package web

import (
	"net/http"
	"testing"
)

func TestHandlePasswordGET_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/password", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandlePasswordGET_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doAuthRequest(t, s, "GET", "/password", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlePasswordPOST_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/password", "current_password=x&new_password=y&confirm_password=y")
	assertRedirect(t, rec, "/login")
}

func TestHandlePasswordPOST_WrongCurrent(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	// cfg.Password is "" — VerifyPassword("wrong", "") returns false
	rec := doAuthFormRequest(t, s, "/password",
		"current_password=wrong&new_password=ValidPassword123&confirm_password=ValidPassword123")
	assertRedirect(t, rec, "/password")
}

func TestHandlePasswordPOST_TooShort(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	// Set a real hash so the current password check passes
	hash, _ := HashPassword("currentpassword123")
	s.cfg.Password = hash

	rec := doAuthFormRequest(t, s, "/password",
		"current_password=currentpassword123&new_password=short&confirm_password=short")
	assertRedirect(t, rec, "/password")
}

func TestHandlePasswordPOST_Mismatch(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword("currentpassword123")
	s.cfg.Password = hash

	rec := doAuthFormRequest(t, s, "/password",
		"current_password=currentpassword123&new_password=ValidPassword123&confirm_password=DifferentPassword123")
	assertRedirect(t, rec, "/password")
}

func TestHandlePasswordPOST_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword("currentpassword123")
	s.cfg.Password = hash

	rec := doAuthFormRequest(t, s, "/password",
		"current_password=currentpassword123&new_password=ValidPassword123&confirm_password=ValidPassword123")
	assertRedirect(t, rec, "/password")
}

// Changing the password ends every other session.
//
// Sessions are a signed cookie with nothing to revoke on the server, so a
// change used to leave anyone already signed in exactly where they were until
// the session timed out — including in the case the change is usually made for.
func TestHandlePasswordPOST_EndsSessionsIssuedUnderTheOldPassword(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, err := HashPassword("currentpassword123")
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.Password = hash

	// Somebody else is signed in, in another browser.
	other := makeAuthCookie(t, s)
	if rec := doRequest(s, "GET", "/dashboard", nil, other); rec.Code == http.StatusSeeOther {
		t.Fatal("the other session is valid before the change")
	}

	rec := doAuthFormRequest(t, s, "/password",
		"current_password=currentpassword123&new_password=ValidPassword123&confirm_password=ValidPassword123")
	assertRedirect(t, rec, "/password")

	after := doRequest(s, "GET", "/dashboard", nil, other)
	if after.Code != http.StatusSeeOther || after.Header().Get("Location") != "/login" {
		t.Errorf("the other session must be refused after the password change, got %d %q",
			after.Code, after.Header().Get("Location"))
	}
}

// The operator making the change stays signed in.
func TestHandlePasswordPOST_KeepsTheChangersOwnSession(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, err := HashPassword("currentpassword123")
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.Password = hash

	cookie := makeAuthCookie(t, s)
	rec := doFormRequest(s, "POST", "/password",
		"current_password=currentpassword123&new_password=ValidPassword123&confirm_password=ValidPassword123",
		cookie)
	assertRedirect(t, rec, "/password")

	// The response re-issues the session; the browser would send that one next.
	refreshed := rec.Result().Cookies()
	if len(refreshed) == 0 {
		t.Fatal("expected a refreshed session cookie")
	}
	if next := doRequest(s, "GET", "/dashboard", nil, refreshed[0]); next.Code == http.StatusSeeOther {
		t.Error("the operator who changed the password must stay signed in")
	}
}
