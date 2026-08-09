package web

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// writeCertValidFor puts a self-signed pair in dir whose certificate expires
// after the given duration.
func writeCertValidFor(t *testing.T, dir string, d time.Duration) *x509.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "easywall"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(d),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"localhost"},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key.pem"),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		t.Fatal(err)
	}

	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func selfSignedManager(t *testing.T, dir string) *certManager {
	t.Helper()
	cfg := &Config{WebConfig: shared.WebConfig{SSLDir: dir}}
	m := newCertManager(cfg)
	t.Cleanup(m.close)
	return m
}

func servedSerial(t *testing.T, m *certManager) *big.Int {
	t.Helper()
	cert, err := m.GetCertificate(nil)
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse served certificate: %v", err)
	}
	return leaf.SerialNumber
}

func TestCertManager_ServesTheGeneratedCertificate(t *testing.T) {
	dir := t.TempDir()
	m := selfSignedManager(t, dir)

	if err := m.ensure(); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cert.pem")); err != nil {
		t.Fatalf("certificate was not generated: %v", err)
	}
	if servedSerial(t, m) == nil {
		t.Error("expected a certificate to be served")
	}
}

// The renewal that mattered: a process running long enough for its own
// certificate to expire. ListenAndServeTLS read the files once at startup, so
// even regenerating them changed nothing until a restart.
func TestCertManager_PicksUpARenewedCertificateWithoutARestart(t *testing.T) {
	dir := t.TempDir()
	writeCertValidFor(t, dir, 10*24*time.Hour) // inside the renewal window
	m := selfSignedManager(t, dir)

	before := servedSerial(t, m)

	// A maintenance tick, as the running server would perform it.
	if err := m.ensure(); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	after := servedSerial(t, m)

	if before.Cmp(after) == 0 {
		t.Error("the renewed certificate must be served without restarting the process")
	}
}

func TestCertManager_LeavesAValidCertificateAlone(t *testing.T) {
	dir := t.TempDir()
	writeCertValidFor(t, dir, 200*24*time.Hour)
	m := selfSignedManager(t, dir)

	before := servedSerial(t, m)
	if err := m.ensure(); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if after := servedSerial(t, m); before.Cmp(after) != 0 {
		t.Error("a certificate nowhere near expiry must not be replaced")
	}
}

func TestCertManager_ReloadsACertificateReplacedOnDisk(t *testing.T) {
	// The ACME case named in the documentation: a custom certificate renewed by
	// something else entirely. easywall reloads it, and never overwrites it.
	dir := t.TempDir()
	writeCertValidFor(t, dir, 90*24*time.Hour)

	cfg := &Config{WebConfig: shared.WebConfig{
		SSLDir: dir,
		TLS: shared.TLSConfig{
			CertFile: filepath.Join(dir, "cert.pem"),
			KeyFile:  filepath.Join(dir, "key.pem"),
		},
	}}
	m := newCertManager(cfg)
	defer m.close()

	before := servedSerial(t, m)

	// Ensure the modification time differs on filesystems with coarse
	// timestamps, then write a different certificate to the same paths.
	time.Sleep(10 * time.Millisecond)
	replaced := writeCertValidFor(t, dir, 90*24*time.Hour)
	_ = os.Chtimes(filepath.Join(dir, "cert.pem"), time.Now().Add(time.Second), time.Now().Add(time.Second))

	after := servedSerial(t, m)
	if before.Cmp(after) == 0 {
		t.Error("a certificate replaced on disk must be picked up on the next handshake")
	}
	if after.Cmp(replaced.SerialNumber) != 0 {
		t.Error("the reloaded certificate is not the one on disk")
	}
}

func TestCertManager_NeverOverwritesACustomCertificate(t *testing.T) {
	dir := t.TempDir()
	expiring := writeCertValidFor(t, dir, time.Hour) // well inside the renewal window

	cfg := &Config{WebConfig: shared.WebConfig{
		SSLDir: dir,
		TLS: shared.TLSConfig{
			CertFile: filepath.Join(dir, "cert.pem"),
			KeyFile:  filepath.Join(dir, "key.pem"),
		},
	}}
	m := newCertManager(cfg)
	defer m.close()

	if err := m.ensure(); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if got := servedSerial(t, m); got.Cmp(expiring.SerialNumber) != 0 {
		t.Error("a certificate the operator supplied must never be replaced by easywall")
	}
}

func TestCertManager_KeepsServingWhenTheFilesGoBad(t *testing.T) {
	dir := t.TempDir()
	m := selfSignedManager(t, dir)
	if err := m.ensure(); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	before := servedSerial(t, m)

	// A half-written replacement, as an interrupted renewal would leave behind.
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), []byte("-----BEGIN CERTIFICATE-----\ntrunc"), 0644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(filepath.Join(dir, "cert.pem"), time.Now().Add(time.Second), time.Now().Add(time.Second))

	if after := servedSerial(t, m); before.Cmp(after) != 0 {
		t.Error("an unreadable replacement must not take the interface down")
	}
}

func TestCertManager_FirstLoadFailureIsReported(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{WebConfig: shared.WebConfig{
		SSLDir: dir,
		TLS: shared.TLSConfig{
			CertFile: filepath.Join(dir, "missing-cert.pem"),
			KeyFile:  filepath.Join(dir, "missing-key.pem"),
		},
	}}
	m := newCertManager(cfg)
	defer m.close()

	if _, err := m.GetCertificate(nil); err == nil {
		t.Error("with nothing to serve, the handshake must fail rather than succeed silently")
	}
}

func TestGenerateSelfSignedCert_WritesBothFilesConsistently(t *testing.T) {
	dir := t.TempDir()
	if err := generateSelfSignedCert(dir); err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("the private key must not be world readable, got %o", perm)
	}

	m := selfSignedManager(t, dir)
	if servedSerial(t, m) == nil {
		t.Error("the generated pair must load as a keypair")
	}
}

// End to end over a real handshake: the certificate a client is shown changes
// after renewal, with no restart in between.
func TestCertManager_ARenewedCertificateReachesTheClient(t *testing.T) {
	dir := t.TempDir()
	writeCertValidFor(t, dir, 10*24*time.Hour)
	m := selfSignedManager(t, dir)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tlsLn := tls.NewListener(ln, &tls.Config{
		GetCertificate: m.GetCertificate,
		MinVersion:     tls.VersionTLS12,
	})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})}
	go func() { _ = srv.Serve(tlsLn) }()
	defer func() { _ = srv.Close() }()

	peerSerial := func() *big.Int {
		conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // self-signed by design
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()
		return conn.ConnectionState().PeerCertificates[0].SerialNumber
	}

	before := peerSerial()
	if err := m.ensure(); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	after := peerSerial()

	if before.Cmp(after) == 0 {
		t.Error("clients still see the expiring certificate after renewal")
	}
}

func TestValidateWebConfig_RejectsHalfConfiguredTLS(t *testing.T) {
	base := func() *Config {
		return &Config{WebConfig: shared.WebConfig{
			BindAddr:   "0.0.0.0:12227",
			SocketPath: "/run/easywall/core.sock",
			SSLDir:     "/etc/easywall/ssl",
			SessionKey: "k",
		}}
	}

	certOnly := base()
	certOnly.TLS.CertFile = "/etc/ssl/fullchain.pem"
	if err := certOnly.Validate(); err == nil {
		t.Error("a certificate without a key must be rejected, not paired with a generated key")
	}

	keyOnly := base()
	keyOnly.TLS.KeyFile = "/etc/ssl/privkey.pem"
	if err := keyOnly.Validate(); err == nil {
		t.Error("a key without a certificate must be rejected")
	}

	both := base()
	both.TLS.CertFile = "/etc/ssl/fullchain.pem"
	both.TLS.KeyFile = "/etc/ssl/privkey.pem"
	if err := both.Validate(); err != nil {
		t.Errorf("a complete custom pair is valid: %v", err)
	}

	if err := base().Validate(); err != nil {
		t.Errorf("neither set means self-signed, which is valid: %v", err)
	}
}
