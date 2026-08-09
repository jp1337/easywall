package core

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestSaveSystemSettings_RoundTrip(t *testing.T) {
	path := writeTempCoreConfig(t, validCoreConfig)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	s := shared.SystemSettings{
		Acceptance: shared.AcceptanceConfig{Enabled: true, Duration: 300},
	}
	if err := cfg.SaveSystemSettings(s); err != nil {
		t.Fatalf("SaveSystemSettings: %v", err)
	}

	cfg2, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig after save: %v", err)
	}
	if !cfg2.Acceptance.Enabled {
		t.Error("expected Acceptance.Enabled=true after reload")
	}
	if cfg2.Acceptance.Duration != 300 {
		t.Errorf("expected Duration=300, got %d", cfg2.Acceptance.Duration)
	}
}

// The core is the authority on what it will store. A duration chosen now is
// refused outright rather than quietly adjusted, so the setting cannot end up
// disagreeing with what the operator was shown.
func TestSaveSystemSettings_RejectsADurationOutsideTheRange(t *testing.T) {
	for _, dur := range []int{0, 1, 9, 3601, 100000} {
		path := writeTempCoreConfig(t, validCoreConfig)
		cfg, err := LoadConfig(path)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}

		err = cfg.SaveSystemSettings(shared.SystemSettings{
			Acceptance: shared.AcceptanceConfig{Enabled: true, Duration: dur},
		})
		if err == nil {
			t.Errorf("duration %d is out of range and must be rejected", dur)
		}

		reloaded, err := LoadConfig(path)
		if err != nil {
			t.Fatalf("LoadConfig after rejected save: %v", err)
		}
		if reloaded.Acceptance.Duration == dur {
			t.Errorf("a rejected duration (%d) must not be written to the config", dur)
		}
	}
}

// A config file already carrying an out-of-range value is a different case: the
// daemon refusing to start would leave the host without a firewall manager over
// a number it can perfectly well bring into range.
func TestValidateCoreConfig_ClampsAnOutOfRangeDuration(t *testing.T) {
	cases := []struct {
		configured, want int
	}{
		{5, shared.AcceptanceDurationMin},
		{9, shared.AcceptanceDurationMin},
		{10, 10},
		{120, 120},
		{3600, 3600},
		{7200, shared.AcceptanceDurationMax},
	}
	for _, tc := range cases {
		cfg := newCoreCfgWith("/run/x", "/data", "/log", tc.configured)
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate(%d): %v", tc.configured, err)
		}
		if cfg.Acceptance.Duration != tc.want {
			t.Errorf("duration %d: expected %d after Validate, got %d",
				tc.configured, tc.want, cfg.Acceptance.Duration)
		}
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

// A config written before 2.5.0 has ipv6.enabled and no ipv6.mode. Both old
// values become "filter": there is no faithful translation of the old
// behaviour, which filtered IPv6 while removing the exemptions it needs, and
// filter is the safe direction as well as what most installations had.
func TestValidate_MigratesIPv6EnabledToMode(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		cfg := newTestConfig(t)
		cfg.IPv6 = shared.IPv6Config{Enabled: enabled}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate(enabled=%v): %v", enabled, err)
		}
		if cfg.IPv6.Mode != shared.IPv6Filter {
			t.Errorf("enabled=%v migrated to %q, want %q", enabled, cfg.IPv6.Mode, shared.IPv6Filter)
		}
	}
}

// An explicit mode is never overwritten by the obsolete boolean.
func TestValidate_ExplicitModeWinsOverTheObsoleteBoolean(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.IPv6 = shared.IPv6Config{Mode: shared.IPv6Block, Enabled: true}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if cfg.IPv6.Mode != shared.IPv6Block {
		t.Errorf("mode became %q", cfg.IPv6.Mode)
	}
}

// A typo must stop the daemon rather than quietly pick a disposition — the
// wrong guess either opens the host or cuts it off.
func TestValidate_RejectsAnUnknownIPv6Mode(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.IPv6 = shared.IPv6Config{Mode: shared.IPv6Mode("passthru")}
	if err := cfg.Validate(); err == nil {
		t.Error("accepted an unknown ipv6.mode")
	}
}

// docker.custom_networks reaches addCIDRAccept, which returns quietly on
// anything it cannot parse — so an unchecked entry was listed in the interface
// as whitelisted and never became a rule.
func TestSaveNetworkSettings_RejectsAnUnparseableDockerNetwork(t *testing.T) {
	cfg := newTestConfig(t)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	err := cfg.SaveNetworkSettings(shared.NetworkSettings{
		IPv6:   shared.IPv6Config{Mode: shared.IPv6Filter},
		Docker: shared.DockerConfig{Enabled: true, CustomNetworks: []string{"172.20.0.0/16", "not-a-network"}},
	})
	if err == nil {
		t.Fatal("accepted a custom network that cannot become a rule")
	}
	if !strings.Contains(err.Error(), "not-a-network") {
		t.Errorf("the error should name the offending entry, got: %v", err)
	}
}

func TestSaveNetworkSettings_RejectsAnUnknownIPv6Mode(t *testing.T) {
	cfg := newTestConfig(t)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := cfg.SaveNetworkSettings(shared.NetworkSettings{
		IPv6: shared.IPv6Config{Mode: shared.IPv6Mode("open")},
	}); err == nil {
		t.Error("accepted an unknown mode; guessing between open and closed is not a choice to make quietly")
	}
}

// SIGHUP reload. features/system-settings.md has always documented it; nothing
// handled the signal, so following the documentation terminated the daemon.
func TestConfigReload_AdoptsTheChangedSections(t *testing.T) {
	path := writeTempCoreConfig(t, validCoreConfig)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	edited := strings.Replace(validCoreConfig, "duration = 120", "duration = 300", 1)
	edited = strings.Replace(edited, "ssh_brute_force = false", "ssh_brute_force = true", 1)
	if err := os.WriteFile(path, []byte(edited), 0600); err != nil {
		t.Fatal(err)
	}

	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := cfg.SystemSettings().Acceptance.Duration; got != 300 {
		t.Errorf("expected the new duration, got %d", got)
	}
	if !cfg.FirewallOptions().SSHBruteForce {
		t.Error("expected the edited firewall option to be adopted")
	}
}

// A typo must never disarm anything: the running configuration stays.
func TestConfigReload_KeepsTheRunningConfigWhenTheFileIsBad(t *testing.T) {
	path := writeTempCoreConfig(t, validCoreConfig)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	before := cfg.SystemSettings().Acceptance.Duration

	for _, bad := range []string{
		"this is not toml {{{",
		validCoreConfig + "\n[ipv6]\nmode = \"sometimes\"\n",
	} {
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if err := cfg.Reload(); err == nil {
			t.Error("a file that does not load or does not validate must be refused")
		}
		if got := cfg.SystemSettings().Acceptance.Duration; got != before {
			t.Errorf("the running configuration changed after a refused reload: %d", got)
		}
	}
}

// The paths are bound at startup. Changing one in the file must not move the
// socket or the rules out from under a running daemon.
func TestConfigReload_IgnoresChangedPaths(t *testing.T) {
	path := writeTempCoreConfig(t, validCoreConfig)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	socketBefore, dataBefore := cfg.SocketPath, cfg.DataDir

	edited := strings.Replace(validCoreConfig, cfg.SocketPath, "/run/somewhere-else.sock", 1)
	edited = strings.Replace(edited, cfg.DataDir, "/var/lib/elsewhere", 1)
	if err := os.WriteFile(path, []byte(edited), 0600); err != nil {
		t.Fatal(err)
	}

	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if cfg.SocketPath != socketBefore || cfg.DataDir != dataBefore {
		t.Errorf("paths must not change on reload: socket=%q data=%q", cfg.SocketPath, cfg.DataDir)
	}
}

// Two configuration saves arriving together must both survive.
//
// The options page and the network page are separate requests, and the daemon
// handles each connection on its own goroutine. Taking a snapshot under a read
// lock and writing it to the file afterwards left the two writes free to
// reorder — an older snapshot reaching the file after a newer one and undoing
// it. Measured at 20 of 100 before the write moved under the same lock as the
// field update.
func TestConfig_ConcurrentSavesDoNotLoseEachOther(t *testing.T) {
	const trials = 100
	lost := 0

	for i := 0; i < trials; i++ {
		path := writeTempCoreConfig(t, validCoreConfig)
		cfg, err := LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := cfg.Validate(); err != nil {
			t.Fatal(err)
		}

		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_ = cfg.SaveFirewallOptions(shared.FirewallOptions{
				SSHBruteForce: true, SSHBruteForceConnectionLimit: 7,
			})
		}()
		go func() {
			defer wg.Done()
			<-start
			_ = cfg.SaveSystemSettings(shared.SystemSettings{
				Acceptance: shared.AcceptanceConfig{Enabled: true, Duration: 300},
			})
		}()
		close(start)
		wg.Wait()

		reloaded, err := LoadConfig(path)
		if err != nil {
			t.Fatalf("the config file did not survive concurrent saves: %v", err)
		}
		if !reloaded.Firewall.SSHBruteForce || reloaded.Acceptance.Duration != 300 {
			lost++
		}
	}

	if lost > 0 {
		t.Errorf("%d of %d concurrent config saves discarded the other change", lost, trials)
	}
}
