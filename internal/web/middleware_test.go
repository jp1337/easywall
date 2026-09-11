package web

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
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
	handler := LoginRateLimit(func(r *http.Request) (string, bool) { return resolveClient(r, nil) }, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	handler := LoginRateLimit(func(r *http.Request) (string, bool) { return resolveClient(r, nil) }, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	handler := LoginRateLimit(func(r *http.Request) (string, bool) { return resolveClient(r, nil) }, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	var proxied []bool
	handler := LoginRateLimit(func(r *http.Request) (string, bool) { return resolveClient(r, nil) },
		func(ip string, p bool) {
			blocked = append(blocked, ip)
			proxied = append(proxied, p)
		})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))

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
	if proxied[0] {
		t.Error("no forwarding header was set; onBlocked reported the request as proxied")
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
	mw := RequireAuth(store, func() string { return credentialFingerprint(current, "", "") })

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	// Sign in under the old password.
	req := httptest.NewRequest("GET", "/dashboard", nil)
	rec := httptest.NewRecorder()
	sess, _ := store.Get(req, SessionName)
	sess.Values[SessionUserKey] = "admin"
	sess.Values[SessionCredentialKey] = credentialFingerprint(oldHash, "", "")
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
	mw := RequireAuth(store, func() string { return credentialFingerprint(hash, "", "") })

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

// resetLoginLimiter empties the process-wide bucket map. The limiter is
// deliberately package-level — one budget per address for the life of the
// process is the point — so tests that share an address have to start clean.
func resetLoginLimiter() {
	loginLimiter.mu.Lock()
	defer loginLimiter.mu.Unlock()
	loginLimiter.buckets = make(map[string]*rateBucket)
}

// Behind a trusted proxy the budget is the client's, not the proxy's. This is
// the shared-budget cost docs-tech/threat-model.md documents, and the reason
// the maintainer chose the full scope over a display-only change.
func TestTheLimiterCountsPerResolvedClient(t *testing.T) {
	resetLoginLimiter()
	t.Cleanup(resetLoginLimiter)

	trusted := []string{"10.1.0.5"}
	resolve := func(r *http.Request) (string, bool) { return resolveClient(r, trusted) }
	h := LoginRateLimit(resolve, nil)(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))

	// Six requests through the trusted proxy, each from a different client.
	// Five is the budget, so a sixth from the *same* client would be refused;
	// six different ones must all pass.
	for i := 0; i < 6; i++ {
		r := httptest.NewRequest("POST", "/login", nil)
		r.RemoteAddr = "10.1.0.5:41000"
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i+1))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d from a distinct client got %d; the budget is still shared",
				i+1, w.Code)
		}
	}

	// The distinct-clients loop above only proves separate budgets exist; it
	// says nothing about whether any one of them is actually capped. Reset so
	// its buckets don't count against this client, then send six requests
	// from the *same* forwarded client: the sixth must be refused, or the
	// trusted branch is handing out an unlimited budget per request.
	resetLoginLimiter()
	for i := 0; i < 6; i++ {
		r := httptest.NewRequest("POST", "/login", nil)
		r.RemoteAddr = "10.1.0.5:41000"
		r.Header.Set("X-Forwarded-For", "198.51.100.9")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if i == 5 && w.Code != http.StatusTooManyRequests {
			t.Errorf("request 6 from the same forwarded client got %d, want 429 — "+
				"the trusted branch is not capping a single client", w.Code)
		}
	}
}

// And an untrusted peer cannot buy itself a fresh budget by rewriting the
// header. This is the bypass the three advisories describe, at the unit level;
// the veth test proves the same against a kernel-assigned peer.
func TestRewritingTheHeaderDoesNotBuyAFreshBudget(t *testing.T) {
	resetLoginLimiter()
	t.Cleanup(resetLoginLimiter)

	resolve := func(r *http.Request) (string, bool) { return resolveClient(r, nil) }
	h := LoginRateLimit(resolve, nil)(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))

	var last int
	for i := 0; i < 6; i++ {
		r := httptest.NewRequest("POST", "/login", nil)
		r.RemoteAddr = "203.0.113.7:5555"
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i+1))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		last = w.Code
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("the sixth attempt got %d, want 429 — a rewritten header bought a new bucket", last)
	}
}

// A trusted proxy with no usable X-Forwarded-For resolves to (peer, true): the
// walk fell back to the peer, so the recorded address stands in for a client it
// could not name. onBlocked must be given that same pair — a regression here
// would silently hide a trusted_proxies-without-proxy_set_header
// misconfiguration in the audit log, reporting a proxy's own address as a
// confirmed client.
func TestOnBlockedReportsProxiedWhenTheTrustedPeerNamesNoClient(t *testing.T) {
	resetLoginLimiter()
	t.Cleanup(resetLoginLimiter)

	trusted := []string{"10.1.0.5"}
	resolve := func(r *http.Request) (string, bool) { return resolveClient(r, trusted) }
	var blockedIP string
	var blockedProxied bool
	h := LoginRateLimit(resolve, func(ip string, p bool) {
		blockedIP, blockedProxied = ip, p
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))

	// No X-Forwarded-For at all: the trusted peer cannot be resolved to a
	// client, so the walk falls back to the peer itself.
	for i := 0; i < 6; i++ {
		r := httptest.NewRequest("POST", "/login", nil)
		r.RemoteAddr = "10.1.0.5:41000"
		h.ServeHTTP(httptest.NewRecorder(), r)
	}

	if blockedIP != "10.1.0.5" {
		t.Errorf("onBlocked was given ip %q, want 10.1.0.5", blockedIP)
	}
	if !blockedProxied {
		t.Error("onBlocked was given proxied=false for a trusted peer with no usable header, want true")
	}
}

// isGatedRoute reports whether route belongs to the set TestTheGateCannotBeWalkedPast
// walks: the authenticated group, not the routes that are reachable by design
// before anyone has signed in.
//
// "/" is not excluded: it sits inside the same RequireAuth+RequireSecondFactor
// group as everything else in server.go, and the gate runs before its handler
// ever gets to redirect to /dashboard. Excluding it would have hidden a real
// hole — the gate skipping the one route whose entire body is "go to the page
// this test exists to protect" — behind a floor number that happened to still
// be met.
//
// /language joins the four routing-table prefixes for the same reason /login
// and /healthz are already there: r.Group registers it, like them, outside
// RequireAuth (see server.go — the login page has to be able to change
// language before a session exists), so it is never reached by
// RequireSecondFactor at all and asserting a gate redirect on it would be
// asserting a behaviour the router never had.
func isGatedRoute(route string) bool {
	switch {
	case route == "/language":
		return false
	case strings.HasPrefix(route, "/login"),
		strings.HasPrefix(route, "/firstrun"),
		strings.HasPrefix(route, "/static"),
		strings.HasPrefix(route, "/healthz"):
		return false
	}
	return true
}

// TestTheGateCannotBeWalkedPast walks every route the server registers and
// asserts that an authenticated session without a second factor reaches none
// of them except the ones enrolment needs.
//
// Every route, from the router's own walk — not a list. A test over a list
// protects the list: the route added next release is not in it, and the test
// stays green while the gate has a hole.
func TestTheGateCannotBeWalkedPast(t *testing.T) {
	s := newFactorTestServer(t, "", 0, false) // no TOTP, no passkey, not demo

	allowed := map[string]bool{
		"/password":                      true,
		"/password/2fa/begin":            true,
		"/password/2fa/confirm":          true,
		"/password/2fa/enrol-unverified": true,
		"/password/2fa/disable":          true,
		"/password/2fa/recovery":         true,
		"/password/passkey/begin":        true,
		"/password/passkey/finish":       true,
		"/password/passkey/remove":       true,
		"/logout":                        true,
	}

	var checked int
	err := chi.Walk(s.router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		// Only the authenticated routes are gated; /login, /firstrun and
		// /static are reachable by design.
		if !isGatedRoute(route) {
			return nil
		}
		checked++
		resp := s.doAuthed(t, method, route)
		defer resp.Body.Close()
		// The discriminator is the gate's own header, not "did this answer land
		// on /password" — an empty-body POST to an enrolment route (a wrong
		// current_password, a short new one, a confirm with no pending secret)
		// legitimately redirects to /password too, for reasons that have
		// nothing to do with the gate. Without the header, every one of those
		// ordinary validation failures reads as "the gate blocked an allowed
		// route" and the assertion below cannot tell the two apart.
		gated := resp.Header.Get("X-Easywall-Gate") != ""
		switch {
		case allowed[route]:
			if gated {
				t.Errorf("%s %s is needed to enrol and the gate redirected it", method, route)
			}
		default:
			if !gated {
				t.Errorf("%s %s answered %d (Location %q) with no gate header; an account with no second factor reached it",
					method, route, resp.StatusCode, resp.Header.Get("Location"))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the router: %v", err)
	}
	// 41 gated routes: the 40 routes chi registers inside the
	// RequireAuth+RequireSecondFactor group — Task 16's own
	// /password/2fa/enrol-unverified and the three /password/passkey/* routes
	// (including the bare "/", whose entire handler is
	// a redirect to the gated /dashboard, and which the gate intercepts
	// before that handler ever runs), plus POST /logout, which isGatedRoute
	// does not exclude (it sits in the public group but is listed in
	// `allowed` above, on purpose — a way out must never need the factor it
	// is gating). A floor copied from the plan (15) would have passed while
	// missing most of the actual group; a floor above the real count would
	// fail on every run for no reason.
	if checked < 41 {
		t.Fatalf("only %d gated routes were walked; the walk is not finding the authenticated group", checked)
	}
}

// TestTheDemoIsExemptAndNothingElseIs asserts the exemption keys on DemoMode.
func TestTheDemoIsExemptAndNothingElseIs(t *testing.T) {
	demo := newFactorTestServer(t, "", 0, true)
	demoResp := demo.doAuthed(t, "GET", "/dashboard")
	defer demoResp.Body.Close()
	if demoResp.StatusCode != 200 {
		t.Errorf("the demo was gated: /dashboard answered %d", demoResp.StatusCode)
	}

	real := newFactorTestServer(t, "", 0, false)
	realResp := real.doAuthed(t, "GET", "/dashboard")
	defer realResp.Body.Close()
	if realResp.StatusCode != 303 {
		t.Errorf("a real installation was not gated: /dashboard answered %d", realResp.StatusCode)
	}
}

// TestTheGateOpensAsSoonAsAFactorExists — one factor is the whole condition.
func TestTheGateOpensAsSoonAsAFactorExists(t *testing.T) {
	for _, tc := range []struct {
		name     string
		totp     string
		passkeys int
	}{
		{"TOTP", "JBSWY3DPEHPK3PXP", 0},
		{"a passkey", "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newFactorTestServer(t, tc.totp, tc.passkeys, false)
			resp := s.doAuthed(t, "GET", "/dashboard")
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Errorf("/dashboard answered %d with %s enrolled", resp.StatusCode, tc.name)
			}
		})
	}
}
