package web

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// renewalWindow is how close to expiry a certificate may come before it is
// replaced.
const renewalWindow = 30 * 24 * time.Hour

// renewalCheckInterval is how often a running server reconsiders its
// certificate. Twice a day is far more often than a yearly certificate needs,
// and cheap: it is a stat and a parse of one small file.
const renewalCheckInterval = 12 * time.Hour

// certManager supplies the TLS certificate for every handshake, and keeps it
// current while the process runs.
//
// Renewal used to happen once, in NewServer. That is fine for a certificate
// with a one-year life only if the process is restarted within the year — and
// easywall-web is a systemd service on a firewall host, which is exactly the
// kind of process that is not. Worse, ListenAndServeTLS reads the files once at
// startup, so even a certificate replaced on disk by hand or by an ACME client
// would not be picked up until a restart. The manager fixes both: it re-reads
// the files when they change, and for the self-signed case it renews them.
type certManager struct {
	certPath string
	keyPath  string

	// sslDir is set only when easywall generates the certificate itself. With a
	// custom certificate, renewal is the operator's business — easywall reloads
	// it but never overwrites it.
	sslDir string

	mu       sync.Mutex
	cert     *tls.Certificate
	loadedAt time.Time // modification time of certPath when cert was loaded

	stop     chan struct{}
	stopOnce sync.Once
}

func newCertManager(cfg *Config) *certManager {
	m := &certManager{
		certPath: cfg.CertPath(),
		keyPath:  cfg.KeyPath(),
		stop:     make(chan struct{}),
	}
	if cfg.TLS.CertFile == "" {
		m.sslDir = cfg.SSLDir
	}
	return m
}

// ensure generates a certificate if easywall owns it and it is missing or
// close to expiry. Called once at startup, and again on every maintenance tick.
func (m *certManager) ensure() error {
	if m.sslDir == "" {
		return nil
	}
	if !certNeedsRenewal(m.certPath) {
		return nil
	}
	slog.Info("generating self-signed TLS certificate", "dir", m.sslDir)
	return generateSelfSignedCert(m.sslDir)
}

// GetCertificate is the tls.Config hook. It returns the loaded certificate,
// re-reading it first if the file on disk has changed since.
func (m *certManager) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cert != nil {
		info, err := os.Stat(m.certPath)
		if err == nil && info.ModTime().Equal(m.loadedAt) {
			return m.cert, nil
		}
	}

	cert, err := tls.LoadX509KeyPair(m.certPath, m.keyPath)
	if err != nil {
		if m.cert != nil {
			// A half-written replacement must not take the interface down;
			// keep serving what already works and try again next handshake.
			slog.Warn("could not load TLS certificate; keeping the previous one",
				"cert", m.certPath, "error", err)
			return m.cert, nil
		}
		return nil, fmt.Errorf("load TLS keypair: %w", err)
	}

	m.cert = &cert
	if info, err := os.Stat(m.certPath); err == nil {
		m.loadedAt = info.ModTime()
	}
	return m.cert, nil
}

// maintain renews the certificate for as long as the server runs.
func (m *certManager) maintain() {
	if m.sslDir == "" {
		return // a custom certificate is renewed by whoever issued it
	}
	ticker := time.NewTicker(renewalCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			if err := m.ensure(); err != nil {
				slog.Error("TLS certificate renewal failed", "error", err)
			}
		}
	}
}

func (m *certManager) close() {
	m.stopOnce.Do(func() { close(m.stop) })
}

// certNeedsRenewal returns true if the cert doesn't exist or expires within 30 days.
func certNeedsRenewal(certPath string) bool {
	// #nosec G304 -- certPath comes from ssl_dir in the config; the operator names
	// the directory, a request never does.
	data, err := os.ReadFile(certPath)
	if err != nil {
		return true
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return true
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return true
	}
	return time.Until(cert.NotAfter) < renewalWindow
}

// generateSelfSignedCert creates a new ECDSA P-256 self-signed certificate valid 1 year.
func generateSelfSignedCert(dir string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"easywall"},
			CommonName:   "easywall",
		},
		NotBefore:   time.Now().Add(-time.Minute),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		// SANs are required by modern browsers — CN alone is not trusted since Chrome 58.
		DNSNames:              []string{"localhost", "easywall"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}

	// The key is written first and both files are replaced by rename, so a
	// handshake never sees a certificate paired with the previous key.
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	if err := writeFileAtomic(dir+"/key.pem", pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		return fmt.Errorf("write key file: %w", err)
	}
	if err := writeFileAtomic(dir+"/cert.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0644); err != nil {
		return fmt.Errorf("write cert file: %w", err)
	}
	return nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}
