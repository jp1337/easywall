package web

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// A certificate a browser believes, fetched and renewed by ACME.
//
// This exists because of passkeys. WebAuthn refuses two things easywall ships
// with by default: a bare IP as the Relying Party ID, and — since Chrome 110 —
// any origin whose certificate the browser does not trust. The second is the
// one a program can fix for its operator, and this is that fix.
//
// HTTP-01 only. TLS-ALPN-01 must be served on port 443, which easywall does not
// listen on and which is usually taken on a host that has a domain pointed at
// it. DNS-01 wants the DNS provider's API credentials in web.toml, and the
// roadmap's Deliberately excluded rejects SMTP on exactly that ground with less
// at stake than a token that can take over the domain.

// acmeCacheDir is where the account key and the issued certificates live,
// under ssl_dir — the one directory the service unit grants write access to
// and deliberately does not mark read-only.
const acmeCacheDir = "acme"

// newACMEManager builds the autocert manager for cfg, or returns an error
// explaining which setting is missing.
//
// It re-checks what Config.validate already refused. That is not redundancy for
// its own sake: the failure it guards is silent. autocert.Prompt is a function
// field, autocert.AcceptTOS is a one-line function returning true, and wiring
// it costs exactly as much as not wiring it — so the check lives next to the
// line it protects as well as at startup.
func newACMEManager(cfg *Config) (*autocert.Manager, error) {
	host := cfg.Hostname()
	if host == "" {
		return nil, fmt.Errorf("tls.acme is on but tls.hostname is empty; a certificate authority issues for a name")
	}
	if !cfg.ACMEAgreedTOS() {
		return nil, fmt.Errorf("tls.acme is on but acme_agree_tos is false; easywall does not agree to " +
			"the certificate authority's subscriber agreement on your behalf")
	}

	m := &autocert.Manager{
		Cache: autocert.DirCache(filepath.Join(cfg.SSLDir, acmeCacheDir)),

		// Exactly one name. Without a policy autocert answers any SNI by
		// asking the CA for a certificate for it, so a stranger connecting
		// with invented names drives this host into the CA's rate limit — and
		// the operator's next real renewal is the one that fails.
		HostPolicy: autocert.HostWhitelist(host),

		Email: cfg.ACMEEmail(),

		// The operator agreed; easywall reports that agreement rather than
		// making it. autocert.AcceptTOS is deliberately not used here — it
		// would say yes for somebody who never read the terms.
		Prompt: func(tosURL string) bool { return true },
	}

	if dir := cfg.ACMEDirectory(); dir != "" {
		m.Client = &acme.Client{DirectoryURL: dir}
	}
	return m, nil
}

// acmeChallengePort is where a certificate authority looks for an HTTP-01
// response. Not configurable — no web.toml key, no flag, nothing an operator
// can reach — because the ACME specification fixes it and a setting for a
// constant is a setting that can only be wrong.
//
// A var regardless, the same reasoning as TelemetryEndpoint and
// telemetryTimeout in internal/shared/telemetry.go: the two arguments are
// independent. "Not configurable" is about the operator-facing surface —
// there is no [tls] key that could disagree with the spec — and says nothing
// about whether test code in this package may point the bind at a different
// port to prove the listener actually opens and actually closes, rather than
// only that its handler answers 404 in isolation. Unexported, so nothing
// outside this package can touch it either way.
var acmeChallengePort = ":80"

// acmeChallengeHandler serves HTTP-01 responses and nothing else.
//
// Deliberately not autocert's own HTTPHandler(nil), whose fallback redirects to
// https://host/ — port 443, where easywall is not listening. An operator who
// typed the bare hostname would be redirected by easywall into a connection
// refused. 404 is the honest answer: this listener exists for one purpose and
// has no opinion about anything else — it is not a second web interface.
func acmeChallengeHandler(m *autocert.Manager) http.Handler {
	return m.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
}

// startACMEChallengeListener opens port 80 for as long as the server runs, or
// does nothing at all when ACME is not configured.
//
// Permanently, not only during issuance: autocert renews at a time it
// chooses, roughly thirty days before expiry, and a listener that is only up
// during a deliberate first issuance is a listener that is down for every
// renewal after it. The failure would be silent for sixty days and then
// total.
func (s *Server) startACMEChallengeListener() error {
	if !s.certs.usesACME() {
		return nil
	}
	// #nosec G102 -- binding every interface is the point, not an oversight.
	// The certificate authority connects from the public internet to prove
	// control of tls.hostname; a listener on 127.0.0.1 or a single configured
	// interface could not answer that challenge at all. What decides whether
	// this ever answers a real request is the same as everywhere else in
	// easywall: the kernel rules, which this process never writes to for
	// port 80 — see port80Reachability in handler_system.go.
	ln, err := net.Listen("tcp", acmeChallengePort) //nolint:gosec // G102 — see above
	if err != nil {
		// Named in full, because the two causes have different fixes and the
		// operator cannot tell them apart from "permission denied".
		return fmt.Errorf("could not listen on port 80 for ACME challenges: %w — "+
			"the service unit needs AmbientCapabilities=CAP_NET_BIND_SERVICE, and nothing "+
			"else on this host may already hold the port", err)
	}
	srv := &http.Server{
		Handler:           acmeChallengeHandler(s.certs.acme),
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.acmeSrv.Store(srv)
	go func() {
		// srv.Close() below is what makes this ErrServerClosed rather than a
		// raw "use of closed network connection" — it flips the server's own
		// done flag, where closing ln directly would not, and every ordinary
		// shutdown would log as though renewal had broken.
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("the ACME challenge listener stopped; certificate renewal will fail", "error", err)
		}
	}()
	slog.Info("listening for ACME challenges", "addr", acmeChallengePort)
	return nil
}

// stopACMEChallengeListener closes the listener, if startACMEChallengeListener
// ever opened one.
func (s *Server) stopACMEChallengeListener() {
	if srv := s.acmeSrv.Load(); srv != nil {
		_ = srv.Close()
	}
}
