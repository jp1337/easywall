package web

import (
	"fmt"
	"path/filepath"

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
