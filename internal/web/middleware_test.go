package web

import (
	"io"
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
		"X-Frame-Options":           "DENY",
		"X-Content-Type-Options":    "nosniff",
		"Referrer-Policy":           "same-origin",
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

func TestSecurityHeaders_NonceInCSPAndContext(t *testing.T) {
	var ctxNonce string
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxNonce, _ = r.Context().Value(nonceCtxKey).(string)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if ctxNonce == "" {
		t.Fatal("nonce not stored in request context")
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "nonce-"+ctxNonce) {
		t.Errorf("CSP does not contain context nonce: CSP=%q nonce=%q", csp, ctxNonce)
	}
}

func TestSecurityHeaders_NonceDiffersPerRequest(t *testing.T) {
	var nonces [2]string
	i := 0
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonces[i], _ = r.Context().Value(nonceCtxKey).(string)
		i++
	}))

	for range nonces {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	if nonces[0] == "" || nonces[1] == "" {
		t.Fatal("expected non-empty nonces")
	}
	if nonces[0] == nonces[1] {
		t.Errorf("expected different nonces per request, both were %q", nonces[0])
	}
}

func TestSecurityHeaders_NoUnsafeInlineInScriptSrc(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if strings.Contains(csp, "'unsafe-inline'") {
		t.Errorf("CSP must not contain 'unsafe-inline': %s", csp)
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
	mw := RequireAuth(store, nil)

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
	mw := RequireAuth(store, nil)

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
	mw := MaxBodySize(10, nil) // 10 bytes max

	var readErr error
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	}))

	body := strings.Repeat("x", 100)
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// The previous version of this test read into a buffer, discarded both the
	// count and the error, and asserted nothing — it passed whatever the
	// middleware did, including doing nothing at all.
	if !isBodyTooLarge(readErr) {
		t.Errorf("reading past the limit must fail with a size error, got %v", readErr)
	}
}

func TestMaxBodySize_SmallBody(t *testing.T) {
	mw := MaxBodySize(1024, nil)

	var got string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		got = string(b)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/", strings.NewReader("small body"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got != "small body" {
		t.Errorf("body under the limit must arrive intact, got %q", got)
	}
}

func TestMaxBodySize_OverriddenPathGetsItsOwnLimit(t *testing.T) {
	mw := MaxBodySize(10, map[string]int64{"/import": 1024})

	var got int
	var readErr error
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		got, readErr = len(b), err
	}))

	body := strings.Repeat("x", 100)
	req := httptest.NewRequest("POST", "/import", strings.NewReader(body))
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if readErr != nil || got != 100 {
		t.Errorf("the override path must accept 100 bytes, read %d bytes with err %v", got, readErr)
	}

	req = httptest.NewRequest("POST", "/ports", strings.NewReader(body))
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if !isBodyTooLarge(readErr) {
		t.Errorf("every other path keeps the default limit, got %v", readErr)
	}
}

func TestLoginRateLimit_Allows(t *testing.T) {
	// Reset global limiter state for this test by using a new IP
	handler := LoginRateLimit(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestLoginRateLimit_RateExceeded(t *testing.T) {
	// Use a unique IP not used by any other test to avoid interference
	const ip = "10.99.200.201"
	handler := LoginRateLimit(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	var lastCode int
	for i := 0; i < 7; i++ {
		req := httptest.NewRequest("POST", "/login", nil)
		req.RemoteAddr = ip + ":9999"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		lastCode = rec.Code
	}
	// After exhausting 5 tokens, request 6+ should be rate-limited
	if lastCode != http.StatusTooManyRequests {
		t.Errorf("expected 429 TooManyRequests after 7 requests, got %d", lastCode)
	}
}

func TestLoginRateLimit_SplitHostPortError(t *testing.T) {
	// RemoteAddr without port — SplitHostPort fails, falls back to full addr
	handler := LoginRateLimit(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "192.0.2.200" // no port — triggers the err != nil branch
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	// Should still allow (first request for this "IP")
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for first request from addr without port, got %d", rec.Code)
	}
}

// login_ratelimited originates in middleware, which must not know CoreClient.
// The callback is what keeps middleware.go free of the client and makes the
// event testable at the same time.
func TestLoginRateLimit_TellsSomebodyWhenItBlocks(t *testing.T) {
	var blocked []string
	handler := LoginRateLimit(func(ip string) { blocked = append(blocked, ip) })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))

	for i := 0; i < 7; i++ {
		req := httptest.NewRequest("POST", "/login", nil)
		req.RemoteAddr = "203.0.113.99:12345"
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
	if len(blocked) == 0 {
		t.Fatal("the limiter refused requests and told nobody; login_ratelimited can never be written")
	}
	if blocked[0] != "203.0.113.99" {
		t.Errorf("onBlocked was given %q, want 203.0.113.99", blocked[0])
	}
}

// A session created under one password must stop working the moment the
// password changes. Sessions live in a signed cookie, so nothing on the server
// expires them: without this check they stayed valid for their full lifetime,
// including the case the change was made for — someone else already signed in.
func TestRequireAuth_SessionFromABeforePasswordChangeIsRejected(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-key-32bytes-padding-padding!"))

	oldHash, err := HashPassword("the-old-password")
	if err != nil {
		t.Fatal(err)
	}
	newHash, err := HashPassword("the-new-password")
	if err != nil {
		t.Fatal(err)
	}

	current := oldHash
	mw := RequireAuth(store, func() string { return credentialFingerprint(current, "") })

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	// Sign in under the old password.
	req := httptest.NewRequest("GET", "/dashboard", nil)
	rec := httptest.NewRecorder()
	sess, _ := store.Get(req, SessionName)
	sess.Values[SessionUserKey] = "admin"
	sess.Values[SessionCredentialKey] = credentialFingerprint(oldHash, "")
	if err := sess.Save(req, rec); err != nil {
		t.Fatal(err)
	}
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	handler.ServeHTTP(httptest.NewRecorder(), req)
	if !called {
		t.Fatal("the session is valid while the password is unchanged")
	}

	// The password changes.
	current = newHash
	called = false
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)

	if called {
		t.Error("a session issued under the previous password must be refused")
	}
	if loc := rec2.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected a redirect to /login, got %q", loc)
	}
}

// A cookie from before this check existed carries no fingerprint at all.
func TestRequireAuth_SessionWithoutAFingerprintIsRejected(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-key-32bytes-padding-padding!"))
	hash, err := HashPassword("some-password")
	if err != nil {
		t.Fatal(err)
	}
	mw := RequireAuth(store, func() string { return credentialFingerprint(hash, "") })

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))

	req := httptest.NewRequest("GET", "/dashboard", nil)
	rec := httptest.NewRecorder()
	sess, _ := store.Get(req, SessionName)
	sess.Values[SessionUserKey] = "admin"
	if err := sess.Save(req, rec); err != nil {
		t.Fatal(err)
	}
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	handler.ServeHTTP(httptest.NewRecorder(), req)
	if called {
		t.Error("a session with no credential fingerprint must be refused")
	}
}
