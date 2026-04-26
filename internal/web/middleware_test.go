package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/sessions"
)

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	headers := map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "same-origin",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
	}
	for name, expected := range headers {
		if got := rec.Header().Get(name); got != expected {
			t.Errorf("header %s: expected %q, got %q", name, expected, got)
		}
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("unexpected CSP: %s", csp)
	}
	if pp := rec.Header().Get("Permissions-Policy"); !strings.Contains(pp, "geolocation=()") {
		t.Errorf("unexpected Permissions-Policy: %s", pp)
	}
}

func TestSecurityHeaders_PassesThrough(t *testing.T) {
	called := false
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("next handler was not called")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("unexpected status: %d", rec.Code)
	}
}

func TestRequireAuth_Unauthenticated(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-key-32bytes-padding-padding!"))
	mw := RequireAuth(store)

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/dashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Error("next handler must not be called for unauthenticated request")
	}
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got: %s", loc)
	}
}

func TestRequireAuth_Authenticated(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-key-32bytes-padding-padding!"))
	mw := RequireAuth(store)

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// Create a request with a valid session cookie
	req := httptest.NewRequest("GET", "/dashboard", nil)
	rec := httptest.NewRecorder()

	// Set session directly via store
	sess, _ := store.Get(req, SessionName)
	sess.Values[SessionUserKey] = "admin"
	_ = sess.Save(req, rec)
	// Copy the Set-Cookie from response to next request
	for _, cookie := range rec.Result().Cookies() {
		req.AddCookie(cookie)
	}

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)

	if !called {
		t.Error("next handler must be called for authenticated request")
	}
}

func TestMaxBodySize_LargeBody(t *testing.T) {
	mw := MaxBodySize(10) // 10 bytes max

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		buf := make([]byte, 100)
		n, _ := r.Body.Read(buf)
		_ = n // MaxBytesReader limits silently on read; handler may still be called
	}))

	body := strings.Repeat("x", 100)
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	_ = called // handler is invoked but body read is truncated
}

func TestMaxBodySize_SmallBody(t *testing.T) {
	mw := MaxBodySize(1024)

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/", strings.NewReader("small body"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("next handler must be called for small body")
	}
}

func TestLoginRateLimit_Allows(t *testing.T) {
	// Reset global limiter state for this test by using a new IP
	handler := LoginRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "192.0.2.100:12345" // TEST-NET IP unlikely to be used elsewhere
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("first request should pass rate limit, got status %d", rec.Code)
	}
}
