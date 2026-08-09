package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

func TestNewServer_Success(t *testing.T) {
	fc := newFakeCore(t)
	dir := t.TempDir()
	sslDir := dir + "/ssl"
	_ = os.MkdirAll(sslDir, 0750)
	cfgPath := dir + "/web.toml"
	_ = os.WriteFile(cfgPath, []byte(`
bind_addr = "127.0.0.1:0"
socket_path = "`+fc.socketPath+`"
ssl_dir = "`+sslDir+`"
data_dir = "`+dir+`"
session_key = "test-session-key-32bytes-padding!"
language = "en"
username = ""
password = ""
[tls]
cert = ""
key  = ""
`), 0600)
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// NewServer should succeed and create SSL certs
	s, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil server")
	}

	// Check SSL cert was generated
	certPath := sslDir + "/cert.pem"
	if _, err := os.Stat(certPath); err != nil {
		t.Errorf("cert.pem not generated: %v", err)
	}
}

func TestNewServer_SSLDirIsFile(t *testing.T) {
	dir := t.TempDir()
	// Create a file at the SSL dir path — os.MkdirAll will fail with ENOTDIR
	sslPath := dir + "/ssl"
	_ = os.WriteFile(sslPath, []byte("not a dir"), 0600)

	cfg := &Config{}
	cfg.BindAddr = "127.0.0.1:0"
	cfg.SocketPath = "/run/x"
	cfg.SSLDir = sslPath
	cfg.SessionKey = "test-key-32bytes-padding-padding!"

	_, err := NewServer(cfg)
	if err == nil {
		t.Error("expected error when SSLDir path is a file")
	}
}

func TestNewServer_CertGenerationError(t *testing.T) {
	dir := t.TempDir()
	sslDir := dir + "/ssl"
	_ = os.MkdirAll(sslDir, 0750)
	// Remove write permission so generateSelfSignedCert fails
	_ = os.Chmod(sslDir, 0555)
	defer func() { _ = os.Chmod(sslDir, 0750) }()

	cfg := &Config{}
	cfg.BindAddr = "127.0.0.1:0"
	cfg.SocketPath = "/run/x"
	cfg.SSLDir = sslDir
	cfg.SessionKey = "test-key-32bytes-padding-padding!"

	_, err := NewServer(cfg)
	if err == nil {
		t.Error("expected error when SSL dir is not writable")
	}
}

func TestNewServer_WithCustomTLSPaths(t *testing.T) {
	fc := newFakeCore(t)
	dir := t.TempDir()
	sslDir := dir + "/ssl"
	_ = os.MkdirAll(sslDir, 0750)

	// Generate a self-signed cert for the test
	if err := generateSelfSignedCert(sslDir); err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}

	cfgPath := dir + "/web.toml"
	_ = os.WriteFile(cfgPath, []byte(`
bind_addr = "127.0.0.1:0"
socket_path = "`+fc.socketPath+`"
ssl_dir = "`+sslDir+`"
data_dir = "`+dir+`"
session_key = "test-session-key-32bytes-padding!"
language = "en"
username = ""
password = ""
[tls]
cert = "`+sslDir+`/cert.pem"
key  = "`+sslDir+`/key.pem"
`), 0600)
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	s, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer with custom TLS: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestLoadTemplates_StubTFunction(t *testing.T) {
	// loadTemplates registers a stub T so ParseGlob succeeds before any request
	// exists. Executing a template without going through render() — which swaps in
	// the per-request T — is what exercises that stub.
	tmpl, err := loadTemplates("testdata/templates")
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "nonce.html", PageData{Nonce: "stub-probe"}); err != nil {
		t.Fatalf("execute fixture template: %v", err)
	}
	if !strings.Contains(buf.String(), "stub-probe") {
		t.Errorf("unexpected output: %s", buf.String())
	}
}

func TestLoadTemplates_Error(t *testing.T) {
	_, err := loadTemplates("/nonexistent/templates/dir")
	if err == nil {
		t.Error("expected error for non-existent templates dir")
	}
}

func TestLoadTemplates_ValidDir(t *testing.T) {
	// testdata/templates should work
	tmpl, err := loadTemplates("testdata/templates")
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}
	if tmpl == nil {
		t.Error("expected non-nil template")
	}
}

func TestRender_NilTemplates(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	s.tmpl = nil // force nil tmpl

	req := httptest.NewRequest("GET", "/login", nil)
	rec := httptest.NewRecorder()
	s.render(rec, req, "login.html", "login", nil)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 with nil tmpl, got %d", rec.Code)
	}
}

func TestRender_WithFlash(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{Active: true}))

	// First: set a flash via a POST that fails
	cookie := makeAuthCookie(t, s)

	// Set flash in the session directly
	req1 := httptest.NewRequest("GET", "/dashboard", nil)
	req1.AddCookie(cookie)
	rec1 := httptest.NewRecorder()
	s.setFlash(rec1, req1, "saved")

	// Get the updated session cookie
	cookies1 := rec1.Result().Cookies()
	if len(cookies1) == 0 {
		t.Fatal("expected session cookie after setFlash")
	}

	// Now make a GET request with that session cookie — render should clear the flash
	req2 := httptest.NewRequest("GET", "/dashboard", nil)
	req2.AddCookie(cookies1[0])
	rec2 := httptest.NewRecorder()
	s.router.ServeHTTP(rec2, req2)
	// Dashboard should render successfully
	assertStatus(t, rec2, http.StatusOK)
}

func TestRender_NonceFromContext(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	// nonce.html exists only as a fixture — it is a probe for the nonce plumbing,
	// not part of the interface, so it is not in web/templates.
	tmpl, err := loadTemplates("testdata/templates")
	if err != nil {
		t.Fatalf("load fixture templates: %v", err)
	}
	s.tmpl = tmpl

	const testNonce = "abc123testNONCEvalue"
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), nonceCtxKey, testNonce))

	rec := httptest.NewRecorder()
	s.render(rec, req, "nonce.html", "test", nil)

	if body := rec.Body.String(); !strings.Contains(body, testNonce) {
		t.Errorf("render did not propagate nonce to template; body=%q", body)
	}
}

// A template that cannot be rendered is a server error and has to be reported
// as one. Executing straight into the ResponseWriter committed a 200 first, so
// the failure arrived as an empty page that looked like a successful one — and
// this test asserted nothing, so it passed either way.
func TestRender_TemplateErrorIsAServerError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	s.render(rec, req, "nonexistent-template.html", "test", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for a template that does not exist, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "<html") {
		t.Error("no partial page may be written before the error is known")
	}
}

// The same for a fragment: htmx swaps the body into the page, so half a
// fragment is swapped in as though it were the whole one.
func TestRenderPartial_TemplateErrorIsAServerError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	req := httptest.NewRequest("POST", "/iplist/validate", nil)
	rec := httptest.NewRecorder()
	s.renderPartial(rec, req, "no-such-partial", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for a fragment that does not exist, got %d", rec.Code)
	}
}

func TestSetFlash(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	s.setFlash(rec, req, "test-flash-message")

	if len(rec.Result().Cookies()) == 0 {
		t.Error("expected session cookie after setFlash")
	}
}

func TestBuildRouter_RegistersRoutes(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	// Verify static files handler exists
	req := httptest.NewRequest("GET", "/static/style.css", nil)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	// Not found is OK (no static files in test), but should not redirect to login
	if rec.Code == http.StatusSeeOther && rec.Header().Get("Location") == "/login" {
		t.Error("static file request should not be redirected to login")
	}
}

func TestBuildRouter_CSRFDenyHandler(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	// A POST with Sec-Fetch-Site: cross-site is rejected by CrossOriginProtection.
	req := httptest.NewRequest("POST", "/login", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for cross-origin POST, got %d", rec.Code)
	}
}

func TestConfigLocalesDir(t *testing.T) {
	cfg := &Config{}
	if d := cfg.LocalesDir(); d != "locales" {
		t.Errorf("expected 'locales', got %q", d)
	}
}

func TestConfigTemplatesDir(t *testing.T) {
	cfg := &Config{}
	if d := cfg.TemplatesDir(); d != "web/templates" {
		t.Errorf("expected 'web/templates', got %q", d)
	}
}

func TestConfigStaticDir(t *testing.T) {
	cfg := &Config{}
	if d := cfg.StaticDir(); d != "web/static" {
		t.Errorf("expected 'web/static', got %q", d)
	}
}

func TestConfigKeyPath_Default(t *testing.T) {
	cfg := &Config{}
	cfg.SSLDir = "/etc/easywall/ssl"
	if kp := cfg.KeyPath(); kp != "/etc/easywall/ssl/key.pem" {
		t.Errorf("expected default key path, got %q", kp)
	}
}

func TestConfigVersionCachePath_NoDataDir(t *testing.T) {
	cfg := &Config{}
	cfg.SSLDir = "/etc/easywall/ssl"
	vcp := cfg.VersionCachePath()
	if vcp == "" {
		t.Error("expected non-empty VersionCachePath when DataDir is empty")
	}
}

// An unusable session key is replaced rather than rejected — refusing to start
// would break `docker compose up` out of the box, and an operator told to edit
// a file inside a container image reaches for the fastest way past the message.
// What must never happen is running with one.
func TestValidate_ReplacesAnUnusableSessionKey(t *testing.T) {
	for _, unusable := range []string{
		"",
		"CHANGE_ME_32_BYTES_HEX_ENCODED_SESSION_SECRET_HERE_XXXXXXXX",
		"short",
	} {
		cfg := newCfgWith("0.0.0.0:12227", "/run/x", "/tmp", unusable)
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate(%q): %v", unusable, err)
		}
		if cfg.SessionKey == unusable {
			t.Errorf("%q was kept as the signing key", unusable)
		}
		if len(cfg.SessionKey) < minSessionKeyLen {
			t.Errorf("the generated key is only %d characters", len(cfg.SessionKey))
		}
		if strings.Contains(cfg.SessionKey, sessionKeyPlaceholder) {
			t.Error("the generated key still contains the placeholder")
		}
	}
}

// A key the operator chose is left alone.
func TestValidate_KeepsAConfiguredSessionKey(t *testing.T) {
	const mine = "1f0c1a3e5b7d9f11335577991bbddff11f0c1a3e5b7d9f11335577991bbddff1"
	cfg := newCfgWith("0.0.0.0:12227", "/run/x", "/tmp", mine)
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.SessionKey != mine {
		t.Error("a configured session key must not be replaced")
	}
}

// And the replacement is persisted, so sessions survive a restart.
func TestValidate_PersistsTheGeneratedSessionKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "web.toml")
	if err := os.WriteFile(path, []byte(`
bind_addr = "0.0.0.0:12227"
socket_path = "/run/easywall/core.sock"
ssl_dir = "/etc/easywall/ssl"
session_key = "CHANGE_ME_32_BYTES_HEX_ENCODED_SESSION_SECRET_HERE_XXXXXXXX"
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.SessionKey != cfg.SessionKey {
		t.Error("the generated key was not written back, so every restart would sign with a different one")
	}
	if strings.Contains(reloaded.SessionKey, sessionKeyPlaceholder) {
		t.Error("the placeholder is still in the file")
	}
}
func TestSaveCredentials_InvalidPath(t *testing.T) {
	cfg := &Config{}
	cfg.configPath = "/nonexistent/path/web.toml"
	err := cfg.SaveCredentials("admin", "hash")
	if err == nil {
		t.Error("expected error for invalid config path")
	}
}

func TestSaveConfig_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "web.toml")
	cfg := &Config{}
	cfg.configPath = cfgPath
	cfg.BindAddr = "0.0.0.0:12227"
	cfg.SocketPath = "/run/x"
	cfg.SSLDir = "/tmp/ssl"
	cfg.SessionKey = "test-key"
	cfg.Language = "en"

	if err := cfg.saveLocked(); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify the file was created
	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}

func TestServer_Stop_NeverStarted(t *testing.T) {
	fc := newFakeCore(t)
	dir := t.TempDir()
	sslDir := dir + "/ssl"
	_ = os.MkdirAll(sslDir, 0750)
	if err := generateSelfSignedCert(sslDir); err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}

	cfg := &Config{}
	cfg.configPath = dir + "/web.toml"
	cfg.BindAddr = "127.0.0.1:0"
	cfg.SocketPath = fc.socketPath
	cfg.SSLDir = sslDir
	cfg.SessionKey = "test-key-32bytes-padding-padding!"

	s, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	// Stop without ever calling Start — must not panic
	s.Stop()
}

func TestServer_StartStop(t *testing.T) {
	fc := newFakeCore(t)
	dir := t.TempDir()
	sslDir := dir + "/ssl"
	_ = os.MkdirAll(sslDir, 0750)
	if err := generateSelfSignedCert(sslDir); err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}

	cfg := &Config{}
	cfg.configPath = dir + "/web.toml"
	cfg.BindAddr = "127.0.0.1:0"
	cfg.SocketPath = fc.socketPath
	cfg.SSLDir = sslDir
	cfg.SessionKey = "test-key-32bytes-padding-padding!"

	s, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- s.Start() }()

	// Give the server time to bind the port before stopping
	time.Sleep(50 * time.Millisecond)
	s.Stop()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned unexpected error after Stop: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("Start did not return within 3s after Stop")
	}
}

func TestServer_Start_MissingCert(t *testing.T) {
	fc := newFakeCore(t)
	dir := t.TempDir()
	sslDir := dir + "/ssl"
	_ = os.MkdirAll(sslDir, 0750)
	// Generate cert so NewServer doesn't fail
	if err := generateSelfSignedCert(sslDir); err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}

	cfg := &Config{}
	cfg.configPath = dir + "/web.toml"
	cfg.BindAddr = "127.0.0.1:0"
	cfg.SocketPath = fc.socketPath
	cfg.SSLDir = sslDir
	cfg.SessionKey = "test-key-32bytes-padding-padding!"

	s, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	// Point the certificate manager at a file that does not exist. The server
	// no longer hands paths to ListenAndServeTLS — the certificate comes from
	// the manager on each handshake — so Start checks it can load one before it
	// binds the port, rather than accepting connections it cannot complete.
	s.certs.certPath = dir + "/nonexistent.pem"
	s.certs.cert = nil

	err = s.Start()
	if err == nil {
		t.Error("expected error when TLS cert file doesn't exist")
	}
}
