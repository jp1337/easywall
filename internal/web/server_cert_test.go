package web

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"html/template"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCertNeedsRenewal_MissingFile(t *testing.T) {
	if !certNeedsRenewal("/nonexistent/cert.pem") {
		t.Error("expected renewal=true for missing cert file")
	}
}

func TestCertNeedsRenewal_InvalidPEM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cert.pem")
	_ = os.WriteFile(path, []byte("not a pem file"), 0644)
	if !certNeedsRenewal(path) {
		t.Error("expected renewal=true for invalid PEM")
	}
}

func TestCertNeedsRenewal_FreshCert(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	writeSelfSignedCert(t, certPath, time.Now().Add(365*24*time.Hour))
	if certNeedsRenewal(certPath) {
		t.Error("expected renewal=false for fresh cert")
	}
}

func TestCertNeedsRenewal_ExpiringSoon(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	// Expires in 5 days — within the 30-day window
	writeSelfSignedCert(t, certPath, time.Now().Add(5*24*time.Hour))
	if !certNeedsRenewal(certPath) {
		t.Error("expected renewal=true for cert expiring in 5 days")
	}
}

func TestCertNeedsRenewal_AlreadyExpired(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	writeSelfSignedCert(t, certPath, time.Now().Add(-1*time.Hour))
	if !certNeedsRenewal(certPath) {
		t.Error("expected renewal=true for already expired cert")
	}
}

func TestGenerateSelfSignedCert_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	if err := generateSelfSignedCert(dir); err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if _, err := os.Stat(certPath); err != nil {
		t.Errorf("cert.pem missing: %v", err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Errorf("key.pem missing: %v", err)
	}
}

func TestGenerateSelfSignedCert_ValidCert(t *testing.T) {
	dir := t.TempDir()
	if err := generateSelfSignedCert(dir); err != nil {
		t.Fatal(err)
	}
	certData, err := os.ReadFile(filepath.Join(dir, "cert.pem"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(certData)
	if block == nil {
		t.Fatal("cert.pem is not valid PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	if cert.Subject.CommonName != "easywall" {
		t.Errorf("unexpected CN: %s", cert.Subject.CommonName)
	}
	if time.Until(cert.NotAfter) < 364*24*time.Hour {
		t.Errorf("cert validity too short: %v", cert.NotAfter)
	}
}

func TestGenerateSelfSignedCert_NotRenewedIfFresh(t *testing.T) {
	dir := t.TempDir()
	if err := generateSelfSignedCert(dir); err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(dir, "cert.pem")
	if certNeedsRenewal(certPath) {
		t.Error("freshly generated cert should not need renewal")
	}
}

func TestTemplateFuncs_FlashClass(t *testing.T) {
	funcs := templateFuncs()
	flashClass := funcs["flashClass"].(func(string) string)

	if c := flashClass("saved"); c != "alert-success" {
		t.Errorf("saved should be alert-success, got: %s", c)
	}
	if c := flashClass("password_too_short"); c != "alert-warning" {
		t.Errorf("password_too_short should be alert-warning, got: %s", c)
	}
	if c := flashClass("some_error"); c != "alert-error" {
		t.Errorf("unknown key should be alert-error, got: %s", c)
	}
	if c := flashClass("rules_accepted"); c != "alert-success" {
		t.Errorf("rules_accepted should be alert-success, got: %s", c)
	}
	if c := flashClass("import_success"); c != "alert-success" {
		t.Errorf("import_success should be alert-success, got: %s", c)
	}
}

func TestTemplateFuncs_FlashIcon(t *testing.T) {
	funcs := templateFuncs()
	flashIcon := funcs["flashIcon"].(func(string) template.HTML)

	successIcon := flashIcon("saved")
	warnIcon := flashIcon("password_mismatch")
	errorIcon := flashIcon("save_error")

	if successIcon == warnIcon {
		t.Error("success and warn icons must differ")
	}
	if successIcon == errorIcon {
		t.Error("success and error icons must differ")
	}
	if warnIcon == errorIcon {
		t.Error("warn and error icons must differ")
	}
}

// writeSelfSignedCert creates a minimal self-signed cert for testing certNeedsRenewal.
func writeSelfSignedCert(t *testing.T, path string, notAfter time.Time) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial := big.NewInt(1)
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "test"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              notAfter,
		BasicConstraintsValid: true,
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	f, _ := os.Create(path)
	defer f.Close()
	_ = pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}
