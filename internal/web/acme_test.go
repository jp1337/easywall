package web

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

// acmeTestConfig returns a config with ACME on, terms agreed and a hostname
// set — the minimum newACMEManager needs to build without error. Every test
// in this file that builds a manager wants exactly this baseline.
func acmeTestConfig(t *testing.T) *Config {
	t.Helper()
	cfg := validTestConfig(t)
	cfg.TLS.ACME = true
	cfg.TLS.Hostname = "firewall.example.org"
	cfg.TLS.ACMEAgreeTOS = true
	return cfg
}

// TestACMEManagerRefusesWithoutAgreedTerms is the second half of the guard
// Task 4's config test starts.
//
// The config refuses to start. This asserts that the manager itself would also
// refuse if it were ever built from a config that got past that — because the
// failure mode is silent: autocert.AcceptTOS is a one-line function that
// returns true, and wiring it is exactly as easy as not wiring it.
func TestACMEManagerRefusesWithoutAgreedTerms(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.TLS.ACME = true
	cfg.TLS.Hostname = "firewall.example.org"
	cfg.TLS.ACMEAgreeTOS = false

	if _, err := newACMEManager(cfg); err == nil {
		t.Fatal("a manager was built without agreed terms")
	}
}

// TestACMEManagerIssuesOnlyForTheConfiguredHost asserts the HostPolicy.
//
// Without one, autocert answers a handshake for any SNI by asking the CA for a
// certificate for it — an unauthenticated stranger can then drive this host
// into the CA's rate limit by connecting with invented names.
func TestACMEManagerIssuesOnlyForTheConfiguredHost(t *testing.T) {
	cfg := acmeTestConfig(t)

	m, err := newACMEManager(cfg)
	if err != nil {
		t.Fatalf("newACMEManager: %v", err)
	}
	if err := m.HostPolicy(t.Context(), "firewall.example.org"); err != nil {
		t.Errorf("the configured host was refused: %v", err)
	}
	err = m.HostPolicy(t.Context(), "somebody-elses.example.com")
	if err == nil {
		t.Error("a host nobody configured was accepted; a stranger can drive this into the CA's rate limit")
	}
}

// TestACMEManagerCachesUnderTheDataDirectory asserts the account key and the
// certificate survive a restart. Without a cache autocert re-registers on every
// start, and Let's Encrypt rate-limits new registrations.
func TestACMEManagerCachesUnderTheDataDirectory(t *testing.T) {
	cfg := acmeTestConfig(t)

	m, err := newACMEManager(cfg)
	if err != nil {
		t.Fatalf("newACMEManager: %v", err)
	}
	dir, ok := m.Cache.(autocert.DirCache)
	if !ok {
		t.Fatalf("the cache is %T, not a DirCache; nothing survives a restart", m.Cache)
	}
	if !strings.HasPrefix(string(dir), cfg.SSLDir) {
		t.Errorf("the cache is at %q, outside ssl_dir %q — the service unit will not be able to write it",
			string(dir), cfg.SSLDir)
	}
}

// TestTheChallengeListenerServesOnlyTheChallengePath asserts that the port-80
// listener is not a second web interface.
//
// autocert's own HTTPHandler(nil) redirects everything that is not a
// challenge to https://host/ — port 443, where easywall is not. An operator
// who types the bare hostname would land on a connection refused, from a
// redirect easywall issued.
func TestTheChallengeListenerServesOnlyTheChallengePath(t *testing.T) {
	m, err := newACMEManager(acmeTestConfig(t))
	if err != nil {
		t.Fatalf("newACMEManager: %v", err)
	}
	h := acmeChallengeHandler(m)

	req := httptest.NewRequest("GET", "http://firewall.example.org/dashboard", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusMovedPermanently || rec.Code == http.StatusFound {
		t.Errorf("a non-challenge path was redirected (%d, Location %q); "+
			"autocert's default points at :443 where easywall is not",
			rec.Code, rec.Header().Get("Location"))
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("a non-challenge path answered %d, want 404", rec.Code)
	}
}

// TestTheChallengeListenerDoesNothingWithoutACME asserts
// startACMEChallengeListener is a no-op — no port bound, nothing left to
// close — on every installation that has not turned tls.acme on, which is
// most of them. newTestServer's certManager is a real one (tls.acme unset in
// its fixture config), the same shape NewServer always builds.
func TestTheChallengeListenerDoesNothingWithoutACME(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	if err := s.startACMEChallengeListener(); err != nil {
		t.Fatalf("startACMEChallengeListener: %v", err)
	}
	if s.acmeSrv.Load() != nil {
		t.Error("a listener was started without ACME configured")
	}
	s.stopACMEChallengeListener() // must not panic with nothing to stop
}

// substringErrorHandler is countingHandler from internal/core's
// appliedconfig_test.go, reproduced here rather than shared across packages
// for one test: it counts Error-level log records whose message contains
// substr, so a test can assert a specific line was — or was not — written,
// instead of parsing captured stdout.
//
// n is written from the Serve goroutine's own logging call and read from the
// test's polling loop — an *atomic.Int64, not a plain int, because the two
// goroutines really do touch it concurrently and a plain int is only "safe"
// by the accident of the write never firing in the passing case. That is
// exactly the case a future regression changes.
type substringErrorHandler struct {
	n      *atomic.Int64
	substr string
}

func (h substringErrorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelError
}
func (h substringErrorHandler) Handle(_ context.Context, r slog.Record) error {
	if strings.Contains(r.Message, h.substr) {
		h.n.Add(1)
	}
	return nil
}
func (h substringErrorHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h substringErrorHandler) WithGroup(string) slog.Handler      { return h }

// TestTheChallengeListenerActuallyBindsAndServes proves the wiring end to
// end, not just its pieces in isolation.
//
// TestTheChallengeListenerServesOnlyTheChallengePath exercises the handler
// directly through httptest — it never calls net.Listen, so it cannot catch a
// missing `go` before srv.Serve (the listener would then hang forever
// unserved), a wrong network or address literal, or a shutdown that leaves
// the port held rather than releasing it. This test dials the real, live
// listener startACMEChallengeListener opened, then confirms
// stopACMEChallengeListener released the port.
//
// It also asserts on the log, and that half exists for a specific reason:
// closing the raw net.Listener directly (rather than the *http.Server, this
// task's own self-review finding) still releases the port — the OS frees a
// listening socket's address on Close regardless of which handle did the
// closing — so a rebind-succeeds check alone cannot tell the two
// implementations apart. What differs is that Serve() then returns a plain
// "use of closed network connection" instead of http.ErrServerClosed, which
// the code logs as "certificate renewal will fail" on every ordinary
// shutdown. Verified below by temporarily reintroducing exactly that bug and
// confirming this handler catches it (rebind still succeeds; the log line
// still fires):
//
//	go test ./internal/web/ -run TestTheChallengeListenerActuallyBindsAndServes -v
//	--- PASS (with the fix)
//	--- with s.acmeSrv.Close() replaced by closing the raw net.Listener:
//	    the "certificate renewal will fail" line appears and this test fails.
//
// Overrides acmeChallengePort rather than the real :80 — see that var's own
// comment for why a fixed-by-the-spec value and a test seam are not the same
// argument. A fixed high port, not :0: the test needs to dial the exact
// address the listener is on, and startACMEChallengeListener does not (and
// should not, in production) hand the bound address back to its caller.
func TestTheChallengeListenerActuallyBindsAndServes(t *testing.T) {
	orig := acmeChallengePort
	acmeChallengePort = ":18443"
	t.Cleanup(func() { acmeChallengePort = orig })

	var falseAlarms atomic.Int64
	prevLog := slog.Default()
	slog.SetDefault(slog.New(substringErrorHandler{n: &falseAlarms, substr: "certificate renewal will fail"}))
	t.Cleanup(func() { slog.SetDefault(prevLog) })

	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	s.cfg.TLS.ACME = true
	s.cfg.TLS.Hostname = "firewall.example.org"
	s.cfg.TLS.ACMEAgreeTOS = true
	certs, err := newCertManager(s.cfg)
	if err != nil {
		t.Fatalf("newCertManager: %v", err)
	}
	s.certs = certs

	if err := s.startACMEChallengeListener(); err != nil {
		t.Fatalf("startACMEChallengeListener: %v", err)
	}
	if s.acmeSrv.Load() == nil {
		t.Fatal("no listener recorded as started")
	}

	resp, err := http.Get("http://127.0.0.1" + acmeChallengePort + "/not-a-challenge")
	if err != nil {
		t.Fatalf("could not reach the listener startACMEChallengeListener opened: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("the live listener answered %d, want 404", resp.StatusCode)
	}

	s.stopACMEChallengeListener()

	// The proof that Stop actually released the port, not merely that the
	// code says it should: if it did not, this second bind on the identical
	// address fails.
	ln, err := net.Listen("tcp", acmeChallengePort)
	if err != nil {
		t.Fatalf("port %s is still held after stopACMEChallengeListener: %v", acmeChallengePort, err)
	}
	_ = ln.Close()

	// The Serve goroutine's own error check runs the instant Close() unblocks
	// its Accept() call, which already happened above (the rebind could not
	// have succeeded otherwise) — this only waits out the scheduler.
	deadline := time.Now().Add(300 * time.Millisecond)
	for falseAlarms.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if falseAlarms.Load() != 0 {
		t.Error(`a clean Stop logged "certificate renewal will fail" — ` +
			"an operator would learn to ignore this error class on every ordinary restart")
	}
}

// TestStartWithACMEConfiguredDoesNotPanic proves Start() actually starts when
// ACME is correctly configured — not merely that a misconfigured setup is
// refused (TestACMEIsRefusedWithoutAgreedTerms and its siblings in
// config_test.go), which is a different claim. Nothing anywhere in this
// package called Start() at all before this test, ACME or not — that gap is
// why a startup-fatal defect here passed review twice.
//
// The defect: Start()'s preflight calls s.certs.GetCertificate(nil) to catch
// a missing certificate file before binding, so the interface refuses to
// start rather than serving a broken handshake to everyone. certManager
// forwards that nil straight into autocert.Manager.GetCertificate, whose
// first statement reads hello.ServerName — a nil-pointer dereference, before
// startACMEChallengeListener (this task's own function, three lines later)
// ever runs. On a real host: three panic-restart cycles against
// StartLimitBurst=3, then no web interface, silently.
//
// Restoring the bug (removing the usesACME() guard around the preflight in
// Start()) makes this test fail:
//
//	go test ./internal/web/ -run TestStartWithACMEConfiguredDoesNotPanic -v
//	--- FAIL: TestStartWithACMEConfiguredDoesNotPanic
//	    acme_test.go:...: Start() panicked with ACME correctly configured:
//	    runtime error: invalid memory address or nil pointer dereference
//
// The goroutine below recovers the panic itself specifically so this shows up
// as a reported FAIL rather than taking the whole test binary down with it —
// go test cannot turn an unrecovered panic in a background goroutine into a
// clean failure of one test.
//
// Built through NewServer, not newTestServer/newACMESystemTestServer: those
// build a Server by hand for handler-level tests that only ever go through
// s.router directly, and leave s.httpSrv nil — a gap that never mattered
// until a test tried to call the real Start(), which panics on a nil
// receiver exactly the way s.certs used to. NewServer is what production
// calls, and is what Start() has to be tested against.
func TestStartWithACMEConfiguredDoesNotPanic(t *testing.T) {
	origPort := acmeChallengePort
	acmeChallengePort = ":18446"
	t.Cleanup(func() { acmeChallengePort = origPort })

	fc := newFakeCore(t)
	dir := t.TempDir()
	sslDir := dir + "/ssl"
	if err := os.MkdirAll(sslDir, 0750); err != nil {
		t.Fatal(err)
	}
	cfgPath := dir + "/web.toml"
	if err := os.WriteFile(cfgPath, []byte(`
bind_addr = "127.0.0.1:19876"
socket_path = "`+fc.socketPath+`"
ssl_dir = "`+sslDir+`"
data_dir = "`+dir+`"
session_key = "test-session-key-32bytes-padding!"
language = "en"
username = "admin"
password = ""
update_check = false
[tls]
cert = ""
key  = ""
hostname = "firewall.example.org"
acme = true
acme_agree_tos = true
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	hash, err := HashPassword(testPassword)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	cfg.Password = hash

	s, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	// TemplatesDir() ("web/templates") is resolved relative to the working
	// directory, which under `go test` is this package's own directory, not
	// the repo root NewServer's real callers run from. A test-only
	// substitution for that path difference, not a stand-in for what
	// NewServer itself builds — everything else above came from NewServer
	// unmodified, httpSrv included.
	s.tmpl = testTemplates(t)
	t.Cleanup(s.Stop)

	type result struct {
		err      error
		panicVal any
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{panicVal: r}
			}
		}()
		done <- result{err: s.Start()}
	}()

	// Poll for the HTTPS port to come up, rather than sleep-and-hope: proof
	// Start() ran all the way through the preflight, the challenge listener,
	// and into ListenAndServeTLS, not just that nothing crashed within an
	// arbitrary window.
	addr := s.cfg.BindAddr
	deadline := time.Now().Add(2 * time.Second)
	dialed := false
	for time.Now().Before(deadline) {
		select {
		case res := <-done:
			if res.panicVal != nil {
				t.Fatalf("Start() panicked with ACME correctly configured: %v", res.panicVal)
			}
			t.Fatalf("Start() returned before ever serving: %v", res.err)
		default:
		}
		conn, dialErr := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			dialed = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !dialed {
		t.Fatal("the HTTPS listener never came up")
	}

	s.Stop()
	select {
	case res := <-done:
		if res.panicVal != nil {
			t.Fatalf("Start() panicked during shutdown: %v", res.panicVal)
		}
		if res.err != nil {
			t.Errorf("Start() returned %v after a clean Stop, want nil", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return after Stop()")
	}
}

// TestACMEManagerNeedsAHostname is the manager-level twin of
// TestACMENeedsAHostname, which asserts the config refusal.
//
// Both halves are needed for the same reason TestACMEManagerRefusesWithoutAgreedTerms
// gives for the terms: the config refuses to start, and this asserts the
// manager would also refuse if it were ever built from a config that got past
// that. An autocert.Manager with no HostPolicy answers any SNI by asking the CA
// for a certificate for it.
func TestACMEManagerNeedsAHostname(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.TLS.ACME = true
	cfg.TLS.ACMEAgreeTOS = true
	cfg.TLS.Hostname = ""

	if _, err := newACMEManager(cfg); err == nil {
		t.Fatal("a manager was built with no hostname; autocert would answer any SNI")
	}
}

// TestACMEManagerReportsTheOperatorsAgreement asserts Prompt returns true.
//
// It is one line in acme.go and it decides whether any certificate is ever
// issued: autocert calls Prompt with the subscriber agreement's URL and
// abandons the order if it returns false. A Prompt that returned false would
// fail every issuance on every installation, and today only the integration
// test — the one that cannot run without CAP_SYS_ADMIN — would notice.
//
// The reason it is written as a closure rather than autocert.AcceptTOS is in
// newACMEManager's own comment: easywall reports the operator's agreement,
// recorded in acme_agree_tos, rather than making it on their behalf.
func TestACMEManagerReportsTheOperatorsAgreement(t *testing.T) {
	cfg := acmeTestConfig(t)

	m, err := newACMEManager(cfg)
	if err != nil {
		t.Fatalf("newACMEManager: %v", err)
	}
	if m.Prompt == nil {
		t.Fatal("Prompt is nil; autocert refuses every order without one")
	}
	if !m.Prompt("https://letsencrypt.org/documents/LE-SA-v1.5-February-24-2025.pdf") {
		t.Error("Prompt returned false — every certificate order on every installation fails")
	}
}
