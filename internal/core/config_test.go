package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

func writeTempCoreConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "easywall.toml")
	if err := os.WriteFile(path, []byte(content), 0640); err != nil {
		t.Fatal(err)
	}
	return path
}

const validCoreConfig = `
socket_path = "/run/easywall/core.sock"
data_dir    = "/var/lib/easywall"
log_dir     = "/var/log/easywall"

[acceptance]
enabled  = true
duration = 120

[firewall]
ssh_brute_force = false
`

func TestLoadCoreConfig_Valid(t *testing.T) {
	path := writeTempCoreConfig(t, validCoreConfig)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.SocketPath != "/run/easywall/core.sock" {
		t.Errorf("unexpected socket_path: %s", cfg.SocketPath)
	}
}

func TestLoadCoreConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path.toml")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLoadCoreConfig_InvalidTOML(t *testing.T) {
	path := writeTempCoreConfig(t, "this = [is not valid toml }")
	_, err := LoadConfig(path)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func newCoreCfgWith(socketPath, dataDir, logDir string, acceptanceDuration int) *Config {
	return &Config{
		CoreConfig: shared.CoreConfig{
			SocketPath: socketPath,
			DataDir:    dataDir,
			LogDir:     logDir,
			Acceptance: shared.AcceptanceConfig{
				Enabled:  true,
				Duration: acceptanceDuration,
			},
		},
	}
}

func TestValidateCoreConfig_MissingSocketPath(t *testing.T) {
	cfg := newCoreCfgWith("", "/data", "/log", 120)
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "socket_path") {
		t.Errorf("expected socket_path error, got: %v", err)
	}
}

func TestValidateCoreConfig_MissingDataDir(t *testing.T) {
	cfg := newCoreCfgWith("/run/x", "", "/log", 120)
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "data_dir") {
		t.Errorf("expected data_dir error, got: %v", err)
	}
}

func TestValidateCoreConfig_MissingLogDir(t *testing.T) {
	cfg := newCoreCfgWith("/run/x", "/data", "", 120)
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "log_dir") {
		t.Errorf("expected log_dir error, got: %v", err)
	}
}

func TestValidateCoreConfig_InvalidAcceptanceDuration(t *testing.T) {
	cfg := newCoreCfgWith("/run/x", "/data", "/log", 0)
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "acceptance") {
		t.Errorf("expected acceptance error, got: %v", err)
	}
}

func TestValidateCoreConfig_SetsSSHDefaults(t *testing.T) {
	cfg := newCoreCfgWith("/run/x", "/data", "/log", 120)
	cfg.Firewall.SSHBruteForce = true
	cfg.Firewall.SSHBruteForceConnectionLimit = 0
	_ = cfg.Validate()
	if cfg.Firewall.SSHBruteForceConnectionLimit != 5 {
		t.Errorf("expected SSHBruteForceConnectionLimit default 5, got %d", cfg.Firewall.SSHBruteForceConnectionLimit)
	}
}

func TestValidateCoreConfig_SetsICMPDefaults(t *testing.T) {
	cfg := newCoreCfgWith("/run/x", "/data", "/log", 120)
	cfg.Firewall.ICMPFlood = true
	cfg.Firewall.ICMPFloodConnectionLimit = 0
	_ = cfg.Validate()
	if cfg.Firewall.ICMPFloodConnectionLimit != 10 {
		t.Errorf("expected ICMPFloodConnectionLimit default 10, got %d", cfg.Firewall.ICMPFloodConnectionLimit)
	}
}

func TestValidateCoreConfig_SetsSYNDefaults(t *testing.T) {
	cfg := newCoreCfgWith("/run/x", "/data", "/log", 120)
	cfg.Firewall.SYNFlood = true
	cfg.Firewall.SYNFloodLimit = 0
	_ = cfg.Validate()
	if cfg.Firewall.SYNFloodLimit != 100 {
		t.Errorf("expected SYNFloodLimit default 100, got %d", cfg.Firewall.SYNFloodLimit)
	}
}

func TestAcceptanceDuration(t *testing.T) {
	cfg := newCoreCfgWith("/run/x", "/data", "/log", 60)
	if d := cfg.AcceptanceDuration(); d != 60*time.Second {
		t.Errorf("unexpected duration: %v", d)
	}
}

func TestRulesPath(t *testing.T) {
	cfg := newCoreCfgWith("/run/x", "/var/lib/easywall", "/log", 120)
	if p := cfg.RulesPath(); p != "/var/lib/easywall/rules.json" {
		t.Errorf("unexpected rules path: %s", p)
	}
}

func TestAuditLogPath(t *testing.T) {
	cfg := newCoreCfgWith("/run/x", "/data", "/var/log/easywall", 120)
	if p := cfg.AuditLogPath(); p != "/var/log/easywall/audit.log" {
		t.Errorf("unexpected audit log path: %s", p)
	}
}

func TestVersionCachePath_Core(t *testing.T) {
	cfg := newCoreCfgWith("/run/x", "/var/lib/easywall", "/log", 120)
	if p := cfg.VersionCachePath(); p != "/var/lib/easywall/version_cache.json" {
		t.Errorf("unexpected version cache path: %s", p)
	}
}

func TestValidateCoreConfig_SetsConnectionLimitDefaults(t *testing.T) {
	cfg := newCoreCfgWith("/run/x", "/data", "/log", 120)
	cfg.Firewall.ConnectionLimit = true
	cfg.Firewall.ConnectionLimitMax = 0
	_ = cfg.Validate()
	if cfg.Firewall.ConnectionLimitMax != 100 {
		t.Errorf("expected ConnectionLimitMax default 100, got %d", cfg.Firewall.ConnectionLimitMax)
	}
}

func TestValidateCoreConfig_SetsLogBlockedDefaults(t *testing.T) {
	cfg := newCoreCfgWith("/run/x", "/data", "/log", 120)
	cfg.Firewall.LogBlocked = true
	cfg.Firewall.LogBlockedLimit = 0
	_ = cfg.Validate()
	if cfg.Firewall.LogBlockedLimit != 60 {
		t.Errorf("expected LogBlockedLimit default 60, got %d", cfg.Firewall.LogBlockedLimit)
	}
}

func TestSaveNetworkSettings_RoundTrip(t *testing.T) {
	path := writeTempCoreConfig(t, validCoreConfig)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	ns := shared.NetworkSettings{
		IPv6: shared.IPv6Config{
			Enabled:                        true,
			ICMPAllowRouterAdvertisement:   true,
			ICMPAllowNeighborAdvertisement: false,
		},
		Docker: shared.DockerConfig{
			Enabled:             true,
			AllowBridgeNetworks: true,
			CustomNetworks:      []string{"172.20.0.0/16"},
		},
	}
	if err := cfg.SaveNetworkSettings(ns); err != nil {
		t.Fatalf("SaveNetworkSettings: %v", err)
	}

	cfg2, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig after save: %v", err)
	}
	if !cfg2.IPv6.Enabled {
		t.Error("expected IPv6.Enabled=true after reload")
	}
	if !cfg2.Docker.Enabled {
		t.Error("expected Docker.Enabled=true after reload")
	}
	if len(cfg2.Docker.CustomNetworks) != 1 || cfg2.Docker.CustomNetworks[0] != "172.20.0.0/16" {
		t.Errorf("unexpected CustomNetworks: %v", cfg2.Docker.CustomNetworks)
	}
}

func TestSaveFirewallOptions_RoundTrip(t *testing.T) {
	path := writeTempCoreConfig(t, validCoreConfig)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	opts := shared.FirewallOptions{
		SSHBruteForce:                true,
		SSHBruteForceConnectionLimit: 3,
		ICMPFlood:                    true,
		SYNFloodLimit:                200,
	}
	if err := cfg.SaveFirewallOptions(opts); err != nil {
		t.Fatalf("SaveFirewallOptions: %v", err)
	}

	// Reload and verify
	cfg2, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig after save: %v", err)
	}
	if !cfg2.Firewall.SSHBruteForce {
		t.Error("expected SSHBruteForce=true after reload")
	}
	if cfg2.Firewall.SSHBruteForceConnectionLimit != 3 {
		t.Errorf("expected SSHBruteForceConnectionLimit=3, got %d", cfg2.Firewall.SSHBruteForceConnectionLimit)
	}
}

func TestSaveFirewallOptions_ReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory permissions; skipping read-only dir test")
	}

	path := writeTempCoreConfig(t, validCoreConfig)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Make config dir read-only so CreateTemp fails
	dir := filepath.Dir(path)
	_ = os.Chmod(dir, 0555)
	defer func() { _ = os.Chmod(dir, 0755) }()

	err = cfg.SaveFirewallOptions(shared.FirewallOptions{SSHBruteForce: true})
	if err == nil {
		t.Error("expected error when config dir is not writable")
	}
}

func TestWriteDefaultCoreConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "easywall.toml")
	if err := WriteDefaultCoreConfig(path); err != nil {
		t.Fatalf("WriteDefaultCoreConfig: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig after WriteDefault: %v", err)
	}
	if cfg.SocketPath != "/run/easywall/core.sock" {
		t.Errorf("unexpected socket_path: %s", cfg.SocketPath)
	}
	if cfg.Acceptance.Duration != 120 {
		t.Errorf("unexpected acceptance duration: %d", cfg.Acceptance.Duration)
	}
}
