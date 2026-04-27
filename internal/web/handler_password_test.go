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
