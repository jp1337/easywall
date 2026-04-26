package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	// Directly execute a loaded template to invoke the stub T function registered
	// during loadTemplates — this covers the stub function body.
	tmpl, err := loadTemplates("testdata/templates")
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	err = tmpl.ExecuteTemplate(&buf, "dashboard.html", PageData{Page: "test"})
	if err != nil {
		t.Fatalf("execute dashboard template: %v", err)
	}
	if !strings.Contains(buf.String(), "test") {
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
	cookie := makeAuthCookie(t, s.store)

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

func TestRender_TemplateError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	// Request a non-existent template name
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	s.render(rec, req, "nonexistent-template.html", "test", nil)
	// Template execution failure is logged but response has already started
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

func TestValidate_MissingSessionKey(t *testing.T) {
	cfg := newCfgWith("0.0.0.0:12227", "/run/x", "/tmp", "")
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty session_key")
	}
}

func TestWriteDefaultWebConfig_InvalidDir(t *testing.T) {
	err := WriteDefaultWebConfig("/nonexistent/path/web.toml")
	if err == nil {
		t.Error("expected error for invalid path")
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

	if err := cfg.save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify the file was created
	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}
