package web

import (
	"strings"
	"testing"

	"golang.org/x/crypto/acme/autocert"
)

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
	cfg := validTestConfig(t)
	cfg.TLS.ACME = true
	cfg.TLS.Hostname = "firewall.example.org"
	cfg.TLS.ACMEAgreeTOS = true

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
	cfg := validTestConfig(t)
	cfg.TLS.ACME = true
	cfg.TLS.Hostname = "firewall.example.org"
	cfg.TLS.ACMEAgreeTOS = true

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
