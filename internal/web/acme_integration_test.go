//go:build integration

package web

// A certificate from a real ACME server, over a real HTTP-01 challenge.
//
// Everything in acme_test.go asserts the shape of the configuration. None of it
// proves an actual certificate ever arrives, and the failures that matter here
// are all at the wire: a challenge served on the wrong path, a HostPolicy that
// refuses the name it was built for, a cache directory the process cannot
// write. Pebble is the ACME server that exists to answer that in a test — the
// same server golang.org/x/crypto/acme's own pebble_test.go uses, copied
// rather than invented (see $(go env GOMODCACHE)/golang.org/x/crypto@*/acme/
// pebble_test.go).
//
// It runs behind the integration tag because it needs a container, exactly as
// the core's nftables suite does, and it reuses that suite's own gate:
// EASYWALL_REQUIRE_SELFTEST turns "no container runtime here" from a skip
// into a failure in CI, the same variable test.yml already sets for the one
// step that runs both ./internal/core/... and ./internal/web/... — a second
// name here would just be a second spelling of the same gate to keep in sync.
//
// The arrangement, and why: two containers, ghcr.io/letsencrypt/pebble (the
// ACME server) and ghcr.io/letsencrypt/pebble-challtestsrv (a DNS stub,
// nothing else — every other protocol it can serve is disabled so it cannot
// collide with the listener under test), both started with --network host
// rather than a bridge network. A bridge would need challtestsrv's
// -defaultIPv4 to name a gateway address podman picks per network, which this
// test would then have to discover; --network host sidesteps that because
// there is no separate network namespace at all — "127.0.0.1" inside either
// container *is* the host's loopback, the same loopback
// startACMEChallengeListener (reached here through the real Start(), not a
// hand-rolled listener) binds acmeChallengePort to. Rootless podman needs no
// extra privilege for --network host: it only skips creating a netns, the
// opposite of the bridge networking the core's suite needs
// CAP_NET_ADMIN/CAP_SYS_ADMIN for. Pebble is told to resolve names against
// challtestsrv (-dnsserver 127.0.0.1:<port>) instead of the real internet, so
// "easywall.test" resolves to 127.0.0.1 — where the real challenge listener
// is — without needing a DNS zone anywhere. PEBBLE_VA_ALWAYS_VALID is left
// unset (default 0): the point of this test is that the challenge is really
// fetched, not that Pebble was told to skip fetching it.

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// requireSelftestEnv is internal/core/selftest_required_test.go's own gate,
// reused rather than reinvented — see that file for the full reasoning, and
// TestTheCIProofGateIsStillWiredUp (internal/shared) for what keeps its
// spelling in test.yml in step with the one there. A skip whose precondition
// is absent reads exactly like a pass; this is what stops that being true in
// CI, for the one precondition this test actually has: a container runtime.
const requireSelftestEnv = "EASYWALL_REQUIRE_SELFTEST"

// skipOrFailUnprovable is the one place this test is allowed to give up on a
// host with no podman or docker. On a developer's machine it skips; in CI,
// with requireSelftestEnv set, it fails — a run that proved nothing must not
// report the same green tick as a run that proved the certificate arrived.
func skipOrFailUnprovable(t *testing.T, reason string) {
	t.Helper()
	if os.Getenv(requireSelftestEnv) != "" {
		t.Fatalf("nothing was proven here, and %s is set: %s\n"+
			"  This job exists to run the proof, so a skip is a failure. If the runner "+
			"image lost its container runtime, fix that — do not unset the variable, "+
			"or the tick goes back to being green for a test that measures nothing.",
			requireSelftestEnv, reason)
	}
	t.Skipf("skipping: %s", reason)
}

// containerRuntime finds podman or docker on PATH. Its absence is the only
// precondition skipOrFailUnprovable exists for here — everything past this
// point (a container that fails to start, an image that will not pull) is a
// real failure and is reported with a plain t.Fatalf, not a skip.
//
// podman first, not merely as a preference: this whole package's TestMain
// (proxy_integration_test.go) re-execs into a fresh network namespace via
// CLONE_NEWNET before any test in this package runs, this one included.
// What matters is not rootless vs. root — this test runs as whatever the
// suite around it does, root in CI (sudo) — but that podman has no
// persistent daemon and forks its container process directly from the
// caller, so a container started with --network host below shares *that*
// namespace — the one this test's own listener is also in. docker is a
// client talking to a system-wide dockerd,
// which normally lives outside TestMain's namespace entirely; a container it
// starts with --network host would share dockerd's namespace instead, not
// this test's, and Pebble would never reach the listener. docker is kept as
// a fallback for a host with no podman rather than removed, since nothing
// else in this file assumes podman specifically — but on a host with only
// docker, this arrangement does not work, and startPebble's own Fatalf on a
// failed container start is what surfaces that rather than a silent skip.
func containerRuntime() (string, error) {
	for _, bin := range []string{"podman", "docker"} {
		if _, err := exec.LookPath(bin); err == nil {
			return bin, nil
		}
	}
	return "", errors.New("neither podman nor docker is on PATH")
}

// pebbleImageVersion pins both images to the version golang.org/x/crypto's
// own pebble_test.go is written against (pebbleModVersion there), rather than
// floating :latest. This is not just for reproducibility: current Pebble
// (confirmed by reading wfe.go at both refs) stopped setting a Location
// header on the finalize-order response — v2.7.0's FinalizeOrder does
// `response.Header().Add("Location", orderURL)`, main's does not — and
// golang.org/x/crypto/acme's own Client.CreateOrderCert reads that header to
// know where to poll while the order is "processing" (finalization runs in a
// background goroutine in current Pebble; even a synchronous success returns
// "processing" first). Losing it makes CreateOrderCert poll an empty URL and
// fail with `Post "": unsupported protocol scheme ""` — confirmed by
// reproducing the identical failure with the bare acme.Client, bypassing
// autocert.Manager entirely, so this is a Pebble/x-crypto version mismatch,
// not anything in this package's own wiring.
const pebbleImageVersion = "2.7.0"

// Fixed rather than dynamically chosen: this test never runs concurrently
// with another copy of itself, and a fixed port lets startPebble address the
// containers directly instead of scraping a log for what they bound.
const (
	pebbleDirPort  = 14000 // pebble's own default ListenAddress
	pebbleMgmtPort = 15000 // pebble's own default ManagementListenAddress

	// pebbleHTTPPort is Pebble's own test-config default (not 80): no choice
	// of port avoids the Host-header mismatch worked around in
	// TestIntegration_ACertificateArrivesOverHTTP01 (see the comment there),
	// since Pebble's va.go always builds the validation URL with
	// net.JoinHostPort(identifier, portString) — port included verbatim,
	// even for 80 — confirmed by trying 80 first and observing the identical
	// failure. Left at Pebble's own default instead, which needs no
	// CAP_NET_BIND_SERVICE.
	pebbleHTTPPort = 5002

	challtestsrvDNSPort = 8053 // pebble-challtestsrv's own default -dnsserver bind
)

// pebbleEnv is what the test needs once both containers are up.
type pebbleEnv struct {
	DirectoryURL string
	caPool       *x509.CertPool // trusts Pebble's own API certificate, not what it issues
	issuingRoots *x509.CertPool // trusts what Pebble issues — a fresh root generated at each startup
}

// TrustingClient returns an *http.Client that trusts Pebble's ACME API
// certificate. That certificate is not a publicly trusted one — Pebble mints
// it from a fixed test CA baked into the image — so the manager's own ACME
// client needs this in place of http.DefaultClient, which would refuse the
// handshake outright.
func (p *pebbleEnv) TrustingClient() *http.Client {
	return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: p.caPool}}}
}

// startPebble brings up challtestsrv and pebble as containers, waits for
// Pebble's directory endpoint, and registers t.Cleanup to tear both down.
func startPebble(t *testing.T) *pebbleEnv {
	t.Helper()

	runtime, err := containerRuntime()
	if err != nil {
		skipOrFailUnprovable(t, err.Error())
		return nil
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	challtestsrvName := "easywall-acme-test-challtestsrv-" + suffix
	pebbleName := "easywall-acme-test-pebble-" + suffix

	cleanupContainer := func(name string) {
		t.Cleanup(func() {
			if t.Failed() {
				out, _ := exec.Command(runtime, "logs", name).CombinedOutput()
				t.Logf("=== %s logs ===\n%s", name, out)
			}
			_ = exec.Command(runtime, "rm", "-f", name).Run()
		})
	}

	// challtestsrv supplies only the DNS piece. -defaultIPv4 is already
	// 127.0.0.1 by default; named explicitly because that address is the
	// whole point of --network host. Every other protocol it can serve is
	// disabled — -http01 would otherwise bind :5002 and race
	// acmeChallengePort for the same port.
	if out, runErr := exec.Command(runtime, "run", "-d", "--network", "host", "--name", challtestsrvName, //nolint:gosec // fixed args, no user input
		"ghcr.io/letsencrypt/pebble-challtestsrv:"+pebbleImageVersion,
		// challtestsrv's own DNS flag, "-dns01" at this pinned version — renamed
		// to "-dnsserver" in later releases (confirmed against the two
		// versions' own -help output). Pebble's *own* "-dnsserver" flag below
		// is unrelated and unchanged; it is Pebble's own client-facing option
		// naming which resolver *it* queries.
		"-dns01", fmt.Sprintf(":%d", challtestsrvDNSPort),
		"-defaultIPv4", "127.0.0.1",
		"-defaultIPv6", "", // no AAAA answers — one address family to reason about
		"-http01", "",
		"-https01", "",
		"-tlsalpn01", "",
		"-doh", "",
	).CombinedOutput(); runErr != nil {
		t.Fatalf("%s run %s: %v\n%s", runtime, challtestsrvName, runErr, out)
	}
	cleanupContainer(challtestsrvName)

	if out, runErr := exec.Command(runtime, "run", "-d", "--network", "host", "--name", pebbleName, //nolint:gosec // fixed args, no user input
		"ghcr.io/letsencrypt/pebble:"+pebbleImageVersion,
		"-config", "test/config/pebble-config.json",
		"-dnsserver", fmt.Sprintf("127.0.0.1:%d", challtestsrvDNSPort),
		"-strict",
	).CombinedOutput(); runErr != nil {
		t.Fatalf("%s run %s: %v\n%s", runtime, pebbleName, runErr, out)
	}
	cleanupContainer(pebbleName)

	dirAddr := fmt.Sprintf("127.0.0.1:%d", pebbleDirPort)
	waitForTCP(t, dirAddr, 20*time.Second)

	// Pebble's own API certificate, read out of the running container rather
	// than vendored: it is a fixed file the image ships
	// (test/certs/pebble.minica.pem), and reading it live means a future
	// pebble image cannot make this stale. `cp`, not `exec cat` — Pebble's
	// image is built FROM scratch (Dockerfile.release copies in one static
	// binary and the test/ directory and nothing else), so there is no
	// shell or coreutils in it to exec at all; `cp` reads the container's
	// filesystem layer from the host side instead.
	caCertPath := filepath.Join(t.TempDir(), "pebble.minica.pem")
	if out, cpErr := exec.Command(runtime, "cp", //nolint:gosec // fixed args, no user input
		pebbleName+":/test/certs/pebble.minica.pem", caCertPath).CombinedOutput(); cpErr != nil {
		t.Fatalf("copy pebble's CA certificate out of the container: %v\n%s", cpErr, out)
	}
	caCertPEM, err := os.ReadFile(caCertPath) // #nosec G304 -- caCertPath is this test's own t.TempDir() path
	if err != nil {
		t.Fatalf("read pebble's CA certificate: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCertPEM) {
		t.Fatal("parse pebble's CA certificate")
	}

	// The issuing hierarchy's root — separate from the API certificate above
	// — is generated fresh on every Pebble startup and published on its
	// management interface. Fetched here so the final dial in the test can
	// verify the issued leaf against it properly, rather than skipping
	// verification and checking the hostname by hand alone.
	mgmtClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	rootResp, err := mgmtClient.Get(fmt.Sprintf("https://127.0.0.1:%d/roots/0", pebbleMgmtPort))
	if err != nil {
		t.Fatalf("fetch pebble's issuing root: %v", err)
	}
	defer func() { _ = rootResp.Body.Close() }()
	rootPEM, err := io.ReadAll(rootResp.Body)
	if err != nil {
		t.Fatalf("read pebble's issuing root: %v", err)
	}
	issuingRoots := x509.NewCertPool()
	if !issuingRoots.AppendCertsFromPEM(rootPEM) {
		t.Fatal("parse pebble's issuing root")
	}

	return &pebbleEnv{
		DirectoryURL: fmt.Sprintf("https://%s/dir", dirAddr),
		caPool:       pool,
		issuingRoots: issuingRoots,
	}
}

func waitForTCP(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s never came up within %s", addr, timeout)
}

// integrationBindAddr is where the HTTPS interface listens for this test —
// fixed, and distinct from the other fixed test ports in this package
// (acme_test.go's :18443/:18446), since this test's own dial has to land on
// the exact port Start() bound.
const integrationBindAddr = "127.0.0.1:19881"

// TestIntegration_ACertificateArrivesOverHTTP01 drives the real Start(), not
// newACMEManager and GetCertificate by hand — that is the path Task 9's
// Critical broke (a nil *tls.ClientHelloInfo straight into
// autocert.Manager.GetCertificate before the challenge listener ever opened),
// and only a test that goes through Start() can catch a regression of it. The
// TLS dial below is what actually triggers issuance: it is the same
// GetCertificate hook production traffic uses, reached through a real
// handshake against the real HTTPS listener, and it cannot complete unless
// startACMEChallengeListener actually opened acmeChallengePort first.
func TestIntegration_ACertificateArrivesOverHTTP01(t *testing.T) {
	pebble := startPebble(t)

	// Set explicitly, even though it happens to match pebbleHTTPPort by
	// construction — the coupling between the two (see that constant's own
	// comment) is worth spelling out at the call site rather than relying on
	// two places agreeing by coincidence.
	origPort := acmeChallengePort
	acmeChallengePort = fmt.Sprintf(":%d", pebbleHTTPPort)
	t.Cleanup(func() { acmeChallengePort = origPort })

	fc := newFakeCore(t)
	dir := t.TempDir()
	sslDir := dir + "/ssl"
	if err := os.MkdirAll(sslDir, 0750); err != nil {
		t.Fatal(err)
	}

	// validTestConfig (config_test.go) is writeTempConfig + LoadConfig under
	// the same fixture acmeTestConfig in acme_test.go already builds on —
	// this is that same shape, with the handful of fields this test needs
	// pointed somewhere real instead of the fixture's own placeholders.
	// Username and UpdateCheckEnabled are left at its defaults, checked as inert rather than carried over — no HTTP request here, and nothing calls Checker.Info().
	cfg := validTestConfig(t)
	cfg.BindAddr = integrationBindAddr
	cfg.SocketPath = fc.socketPath
	cfg.SSLDir = sslDir
	cfg.DataDir = dir
	cfg.TLS.Hostname = "easywall.test"
	cfg.TLS.ACME = true
	cfg.TLS.ACMEAgreeTOS = true
	cfg.TLS.ACMEDirectory = pebble.DirectoryURL

	hash, err := HashPassword(testPassword)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	cfg.Password = hash

	s, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	// Pebble's own API certificate is not a public one; see TrustingClient.
	s.certs.acme.Client.HTTPClient = pebble.TrustingClient()
	// Pebble's own va.go builds every validation URL with
	// net.JoinHostPort(identifier, portString), so Pebble always sends
	// "easywall.test:5002" as the Host header — confirmed by trying port 80
	// first (see pebbleHTTPPort's own comment) and observing the identical
	// failure there too. Go's net/http never strips a redundant default port
	// either; a real client simply never puts one in the URL to begin with,
	// so newACMEManager's autocert.HostWhitelist(host) — exact string match,
	// no port handling, and covered on its own by
	// TestACMEManagerIssuesOnlyForTheConfiguredHost — would 403 every
	// request Pebble ever makes, on any port. That is Pebble's own
	// test-harness shape (built to run on whatever port a test picks), not a
	// gap in production's policy (which only ever has to recognize a bare
	// hostname, because a real client never sends a port at all). Worked
	// around here, after NewServer, rather than by loosening the production
	// policy itself: still exactly one accepted host, with the port
	// Pebble's harness adds stripped before the comparison.
	configuredHost := cfg.Hostname()
	s.certs.acme.HostPolicy = func(_ context.Context, host string) error {
		if h, _, splitErr := net.SplitHostPort(host); splitErr == nil {
			host = h
		}
		if host != configuredHost {
			return fmt.Errorf("host %q not configured", host)
		}
		return nil
	}
	// TemplatesDir() resolves relative to the working directory, which under
	// `go test` is this package's own directory rather than the repo root
	// NewServer's real callers run from — see TestStartWithACMEConfiguredDoesNotPanic
	// for the same substitution.
	s.tmpl = testTemplates(t)
	t.Cleanup(s.Stop)

	done := make(chan error, 1)
	go func() { done <- s.Start() }()

	waitForTCP(t, integrationBindAddr, 5*time.Second)
	select {
	case err := <-done:
		t.Fatalf("Start() returned before ever serving: %v", err)
	default:
	}

	// The trigger: a real TLS handshake with the configured SNI against the
	// real HTTPS listener Start() opened. RootCAs is Pebble's own issuing
	// root (fetched in startPebble), so this dial fully chain-verifies the
	// certificate autocert hands back — a stronger claim than skipping
	// verification and checking the hostname by hand alone.
	//
	// 120s, not the brief's suggested 90s: one local run hit 90s exactly,
	// waiting out Pebble's own randomised per-attempt VA delay (up to 4s ×
	// 3 concurrent attempts, compounding across the tls-alpn-01 attempt
	// autocert always tries first and the http-01 one that actually
	// succeeds). Not a fix for that variance — every wait in this test
	// already carries an explicit deadline, there is no raw sleep to find —
	// just margin, and the outer CI step's own -timeout 480s has room for it.
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	dialer := &tls.Dialer{Config: &tls.Config{ServerName: "easywall.test", RootCAs: pebble.issuingRoots}}
	conn, err := dialer.DialContext(ctx, "tcp", integrationBindAddr)
	if err != nil {
		t.Fatalf("no certificate arrived: %v", err)
	}
	defer func() { _ = conn.Close() }()

	peerCerts := conn.(*tls.Conn).ConnectionState().PeerCertificates
	if len(peerCerts) == 0 {
		t.Fatal("the handshake completed with no certificate")
	}
	leaf := peerCerts[0]
	if err := leaf.VerifyHostname("easywall.test"); err != nil {
		t.Errorf("the issued certificate is not for the configured host: %v", err)
	}

	// And it is on disk, so a restart does not re-register.
	entries, _ := os.ReadDir(filepath.Join(sslDir, acmeCacheDir))
	if len(entries) == 0 {
		t.Error("nothing was cached; every restart would re-register with the CA")
	}
}
