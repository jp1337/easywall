package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "web.toml")
	if err := os.WriteFile(path, []byte(content), 0640); err != nil {
		t.Fatal(err)
	}
	return path
}

const validConfigContent = `
bind_addr    = "0.0.0.0:12227"
socket_path  = "/run/easywall/core.sock"
ssl_dir      = "/tmp/ssl"
session_key  = "a-session-key-that-is-long-enough"
language     = "en"
username     = ""
password     = ""
[tls]
cert = ""
key  = ""
`

func TestLoadConfig_Valid(t *testing.T) {
	path := writeTempConfig(t, validConfigContent)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.BindAddr != "0.0.0.0:12227" {
		t.Errorf("unexpected bind_addr: %s", cfg.BindAddr)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/web.toml")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLoadConfig_InvalidTOML(t *testing.T) {
	path := writeTempConfig(t, "this = [is not valid toml }")
	_, err := LoadConfig(path)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func newCfgWith(bindAddr, socketPath, sslDir, sessionKey string) *Config {
	return &Config{
		WebConfig: shared.WebConfig{
			BindAddr:   bindAddr,
			SocketPath: socketPath,
			SSLDir:     sslDir,
			SessionKey: sessionKey,
		},
	}
}

func TestValidate_MissingBindAddr(t *testing.T) {
	cfg := newCfgWith("", "/run/x", "/tmp", "key")
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "bind_addr") {
		t.Errorf("expected bind_addr error, got: %v", err)
	}
}

func TestValidate_MissingSocketPath(t *testing.T) {
	cfg := newCfgWith("0.0.0.0:12227", "", "/tmp", "key")
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "socket_path") {
		t.Errorf("expected socket_path error, got: %v", err)
	}
}

func TestValidate_MissingSSLDir(t *testing.T) {
	cfg := newCfgWith("0.0.0.0:12227", "/run/x", "", "key")
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "ssl_dir") {
		t.Errorf("expected ssl_dir error, got: %v", err)
	}
}

func TestValidate_DefaultsDataDir(t *testing.T) {
	path := writeTempConfig(t, validConfigContent)
	cfg, _ := LoadConfig(path)
	_ = cfg.Validate()
	if cfg.DataDir != "/var/lib/easywall" {
		t.Errorf("expected default DataDir, got: %s", cfg.DataDir)
	}
}

func TestValidate_DefaultsLanguage(t *testing.T) {
	content := strings.ReplaceAll(validConfigContent, `language     = "en"`, "")
	path := writeTempConfig(t, content)
	cfg, _ := LoadConfig(path)
	_ = cfg.Validate()
	if cfg.Language != "en" {
		t.Errorf("expected default language 'en', got: %s", cfg.Language)
	}
}

func TestIsFirstRun_NoPassword(t *testing.T) {
	path := writeTempConfig(t, validConfigContent)
	cfg, _ := LoadConfig(path)
	if !cfg.IsFirstRun() {
		t.Error("expected IsFirstRun=true when password is empty")
	}
}

func TestIsFirstRun_WithPassword(t *testing.T) {
	content := strings.ReplaceAll(validConfigContent, `password     = ""`, `password = "$argon2id$v=19$m=65536,t=3,p=4$abc$xyz"`)
	path := writeTempConfig(t, content)
	cfg, _ := LoadConfig(path)
	if cfg.IsFirstRun() {
		t.Error("expected IsFirstRun=false when password is set")
	}
}

func TestCertPath_Default(t *testing.T) {
	path := writeTempConfig(t, validConfigContent)
	cfg, _ := LoadConfig(path)
	if !strings.Contains(cfg.CertPath(), "cert.pem") {
		t.Errorf("expected cert.pem in default CertPath, got: %s", cfg.CertPath())
	}
}

func TestCertPath_Custom(t *testing.T) {
	content := strings.ReplaceAll(validConfigContent, `cert = ""`, `cert = "/etc/ssl/my.crt"`)
	path := writeTempConfig(t, content)
	cfg, _ := LoadConfig(path)
	if cfg.CertPath() != "/etc/ssl/my.crt" {
		t.Errorf("unexpected CertPath: %s", cfg.CertPath())
	}
}

func TestKeyPath_Custom(t *testing.T) {
	content := strings.ReplaceAll(validConfigContent, `key  = ""`, `key = "/etc/ssl/my.key"`)
	path := writeTempConfig(t, content)
	cfg, _ := LoadConfig(path)
	if cfg.KeyPath() != "/etc/ssl/my.key" {
		t.Errorf("unexpected KeyPath: %s", cfg.KeyPath())
	}
}

func TestVersionCachePath(t *testing.T) {
	path := writeTempConfig(t, validConfigContent)
	cfg, _ := LoadConfig(path)
	_ = cfg.Validate() // sets DataDir default
	vcp := cfg.VersionCachePath()
	if !strings.Contains(vcp, "version_cache.json") {
		t.Errorf("unexpected VersionCachePath: %s", vcp)
	}
}

func TestWriteDefaultWebConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "web.toml")
	if err := WriteDefaultWebConfig(path); err != nil {
		t.Fatalf("WriteDefaultWebConfig: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig after WriteDefault: %v", err)
	}
	if cfg.BindAddr != "0.0.0.0:12227" {
		t.Errorf("unexpected bind_addr: %s", cfg.BindAddr)
	}
	if len(cfg.SessionKey) == 0 {
		t.Error("session_key must be set in default config")
	}
}

func TestSaveCredentials(t *testing.T) {
	path := writeTempConfig(t, validConfigContent)
	cfg, _ := LoadConfig(path)
	if err := cfg.SaveCredentials("admin", "$argon2id$v=19$m=65536,t=3,p=4$abc$xyz"); err != nil {
		t.Fatal(err)
	}
	// reload and check
	cfg2, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Username != "admin" {
		t.Errorf("expected username 'admin', got: %s", cfg2.Username)
	}
	if cfg2.Password != "$argon2id$v=19$m=65536,t=3,p=4$abc$xyz" {
		t.Errorf("unexpected password hash: %s", cfg2.Password)
	}
}

func TestGenerateSecret_Length(t *testing.T) {
	s, err := generateSecret(32)
	if err != nil {
		t.Fatal(err)
	}
	// hex encoding: 32 bytes → 64 chars
	if len(s) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(s))
	}
}

func TestGenerateSecret_Unique(t *testing.T) {
	s1, _ := generateSecret(16)
	s2, _ := generateSecret(16)
	if s1 == s2 {
		t.Error("two generateSecret calls should produce different results")
	}
}

func TestSave_CreateTempError(t *testing.T) {
	// Directly test save() with a nonexistent directory — os.CreateTemp fails
	cfg := &Config{
		WebConfig: shared.WebConfig{
			BindAddr:   "0.0.0.0:12227",
			SocketPath: "/run/x",
			SSLDir:     "/tmp",
			SessionKey: "key",
		},
		configPath: "/nonexistent/directory/web.toml",
	}
	err := cfg.save()
	if err == nil {
		t.Error("expected error when directory doesn't exist")
	}
	if !strings.Contains(err.Error(), "create temp config") {
		t.Errorf("expected 'create temp config' in error, got: %v", err)
	}
}

func TestSave_AtomicRenameToSameDir(t *testing.T) {
	// Test successful save — os.Rename works within same directory
	dir := t.TempDir()
	path := filepath.Join(dir, "web.toml")
	// Write initial file so save can rename over it
	_ = os.WriteFile(path, []byte(""), 0600)
	cfg := &Config{
		WebConfig: shared.WebConfig{
			BindAddr:   "0.0.0.0:12227",
			SocketPath: "/run/x",
			SSLDir:     dir,
			SessionKey: "secret",
		},
		configPath: path,
	}
	if err := cfg.save(); err != nil {
		t.Fatalf("save should succeed in writable dir: %v", err)
	}
}

func TestWriteDefaultWebConfig_InvalidPath(t *testing.T) {
	err := WriteDefaultWebConfig("/nonexistent/directory/web.toml")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestSave_RenameError(t *testing.T) {
	dir := t.TempDir()
	// Create a directory at the target path — os.Rename(tmp, dir) returns EISDIR
	targetPath := filepath.Join(dir, "web.toml")
	if err := os.Mkdir(targetPath, 0755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	cfg := &Config{
		WebConfig: shared.WebConfig{
			BindAddr:   "0.0.0.0:12227",
			SocketPath: "/run/x",
			SSLDir:     dir,
			SessionKey: "secret",
		},
		configPath: targetPath,
	}
	err := cfg.save()
	if err == nil {
		t.Error("expected error when rename target is a directory")
	}
}

func TestVersionCachePath_NoDataDir(t *testing.T) {
	cfg := &Config{
		WebConfig: shared.WebConfig{
			SSLDir: "/etc/easywall/ssl",
		},
	}
	// DataDir is empty — should use SSLDir + "/../version_cache.json"
	vcp := cfg.VersionCachePath()
	if !strings.Contains(vcp, "version_cache.json") {
		t.Errorf("unexpected VersionCachePath: %s", vcp)
	}
}
