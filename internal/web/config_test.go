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

// A failed write must not leave any of the four savers' in-memory state ahead
// of disk. Each is seeded with a known-good value, pointed at a config path
// whose directory does not exist (saveLocked's os.CreateTemp fails on that
// deterministically, the same technique TestSave_CreateTempError uses), and
// asked to save something different. The save must fail and the seeded value
// must still be exactly what it was — not what the failed call tried to make
// it — the same rollback SaveTOTP already had.
func TestConfigSavers_RollBackOnAFailedWrite(t *testing.T) {
	newCfg := func() *Config {
		return &Config{
			WebConfig: shared.WebConfig{
				BindAddr:      "0.0.0.0:12227",
				SocketPath:    "/run/x",
				SSLDir:        "/tmp",
				SessionKey:    "key",
				Username:      "admin",
				Password:      "$argon2id$original",
				TOTPSecret:    "ORIGINALSECRET",
				RecoveryCodes: []string{"$argon2id$orig1", "$argon2id$orig2"},
			},
			configPath: "/nonexistent/directory/web.toml",
		}
	}

	t.Run("SaveTelemetry", func(t *testing.T) {
		cfg := newCfg()
		on := true
		cfg.Telemetry = &on
		off := false
		if err := cfg.SaveTelemetry(off); err == nil {
			t.Fatal("expected an error from a directory that does not exist")
		}
		if cfg.Telemetry == nil || *cfg.Telemetry != on {
			t.Error("Telemetry changed in memory despite the failed write")
		}
	})

	t.Run("SaveCredentials", func(t *testing.T) {
		cfg := newCfg()
		if err := cfg.SaveCredentials("attacker", "$argon2id$new"); err == nil {
			t.Fatal("expected an error from a directory that does not exist")
		}
		if cfg.Username != "admin" || cfg.Password != "$argon2id$original" {
			t.Errorf("credentials changed in memory despite the failed write: %q / %q",
				cfg.Username, cfg.Password)
		}
	})

	t.Run("SaveRecoveryCodes", func(t *testing.T) {
		cfg := newCfg()
		if err := cfg.SaveRecoveryCodes([]string{"$argon2id$new1"}); err == nil {
			t.Fatal("expected an error from a directory that does not exist")
		}
		want := []string{"$argon2id$orig1", "$argon2id$orig2"}
		got := cfg.WebConfig.RecoveryCodes
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("recovery codes changed in memory despite the failed write: %v", got)
		}
	})

	t.Run("SaveFirstRun", func(t *testing.T) {
		cfg := newCfg()
		cfg.Password = "" // SaveFirstRun requires this to still look like first-run
		telemetry := true
		if err := cfg.SaveFirstRun(FirstRunAccount{
			Username:     "newadmin",
			PasswordHash: "$argon2id$new",
			Telemetry:    telemetry,
		}); err == nil {
			t.Fatal("expected an error from a directory that does not exist")
		}
		if cfg.Password != "" || cfg.Username != "admin" || cfg.Telemetry != nil {
			t.Errorf("first-run fields changed in memory despite the failed write: user=%q pass=%q telemetry=%v",
				cfg.Username, cfg.Password, cfg.Telemetry)
		}
		// SaveFirstRun rolls back TOTPSecret/RecoveryCodes too — the wizard's
		// confirm path writes both alongside the account in the same call, and a
		// failed write must not leave the config believing a factor that was
		// never actually persisted is enrolled.
		if cfg.WebConfig.TOTPSecret != "ORIGINALSECRET" {
			t.Errorf("TOTPSecret changed in memory despite the failed write: %q", cfg.WebConfig.TOTPSecret)
		}
		wantCodes := []string{"$argon2id$orig1", "$argon2id$orig2"}
		gotCodes := cfg.WebConfig.RecoveryCodes
		if len(gotCodes) != len(wantCodes) || gotCodes[0] != wantCodes[0] || gotCodes[1] != wantCodes[1] {
			t.Errorf("recovery codes changed in memory despite the failed write: %v", gotCodes)
		}
	})
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
	err := cfg.saveLocked()
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
	if err := cfg.saveLocked(); err != nil {
		t.Fatalf("save should succeed in writable dir: %v", err)
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
	err := cfg.saveLocked()
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

// The packaged layout puts web.toml in a directory owned by root, because that
// directory also holds easywall.toml — the configuration the root daemon reads.
// Making it writable by the unprivileged web user so it could create a temp file
// there would hand a network-facing process the ability to rewrite what root
// loads. So the save has to work without write access to the directory.
func TestConfigSave_WorksWithoutWriteAccessToTheDirectory(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "web.toml")
	if err := os.WriteFile(path, []byte(`
bind_addr = "0.0.0.0:12227"
socket_path = "/run/easywall/core.sock"
ssl_dir = "/etc/easywall/ssl"
session_key = "key"
username = ""
password = ""
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}

	// The file stays writable; the directory does not.
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })

	hash, err := HashPassword("firstrunpassword123")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveCredentials("admin", hash); err != nil {
		t.Fatalf("the first-run wizard must be able to save its credentials: %v", err)
	}

	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("reload after in-place save: %v", err)
	}
	if reloaded.Username != "admin" || !VerifyPassword("firstrunpassword123", reloaded.Password) {
		t.Error("the credentials did not survive the in-place write")
	}
	// And nothing was left lying about.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("expected only web.toml in the directory, got %d entries", len(entries))
	}
}
