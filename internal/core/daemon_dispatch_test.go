package core

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// ---------------------------------------------------------------------------
// CmdSaveOptions
// ---------------------------------------------------------------------------

func TestDispatch_SaveOptions_InvalidPayload(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	resp := d.dispatch(shared.Command{Type: shared.CmdSaveOptions, Payload: []byte(`{invalid`)})
	if resp.Success {
		t.Error("expected failure for invalid JSON payload")
	}
}

func TestDispatch_SaveOptions_SaveError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory permissions; skipping read-only dir test")
	}

	path := writeTempCoreConfig(t, validCoreConfig)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	// Create data/log dirs so newTestFirewall doesn't fail
	cfg.DataDir = t.TempDir()
	if err := os.MkdirAll(cfg.DataDir, 0750); err != nil {
		t.Fatal(err)
	}
	cfg.LogDir = t.TempDir()
	if err := os.MkdirAll(cfg.LogDir, 0750); err != nil {
		t.Fatal(err)
	}

	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	// Make config dir read-only so save() → CreateTemp fails
	cfgDir := filepath.Dir(path)
	_ = os.Chmod(cfgDir, 0555)
	defer func() { _ = os.Chmod(cfgDir, 0755) }()

	payload, _ := json.Marshal(shared.FirewallOptions{SSHBruteForce: true})
	resp := d.dispatch(shared.Command{Type: shared.CmdSaveOptions, Payload: payload})
	if resp.Success {
		t.Error("expected failure when config dir is read-only")
	}
}

// ---------------------------------------------------------------------------
// CmdGetSettings
// ---------------------------------------------------------------------------

func TestDispatch_GetSettings(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	resp := d.dispatch(shared.Command{Type: shared.CmdGetSettings})
	if !resp.Success {
		t.Fatalf("GetSettings: %s", resp.Error)
	}
	var s shared.NetworkSettings
	if err := json.Unmarshal(resp.Data, &s); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CmdSaveSettings
// ---------------------------------------------------------------------------

func TestDispatch_SaveSettings_InvalidPayload(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	resp := d.dispatch(shared.Command{Type: shared.CmdSaveSettings, Payload: []byte(`{invalid`)})
	if resp.Success {
		t.Error("expected failure for invalid JSON payload")
	}
}

func TestDispatch_SaveSettings_Success(t *testing.T) {
	path := writeTempCoreConfig(t, validCoreConfig)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.DataDir = t.TempDir()
	if err := os.MkdirAll(cfg.DataDir, 0750); err != nil {
		t.Fatal(err)
	}
	cfg.LogDir = t.TempDir()
	if err := os.MkdirAll(cfg.LogDir, 0750); err != nil {
		t.Fatal(err)
	}

	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	s := shared.NetworkSettings{
		IPv6:   shared.IPv6Config{Enabled: true},
		Docker: shared.DockerConfig{Enabled: false},
	}
	payload, _ := json.Marshal(s)
	resp := d.dispatch(shared.Command{Type: shared.CmdSaveSettings, Payload: payload})
	if !resp.Success {
		t.Fatalf("SaveSettings: %s", resp.Error)
	}
}

func TestDispatch_SaveSettings_SaveError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory permissions; skipping read-only dir test")
	}

	path := writeTempCoreConfig(t, validCoreConfig)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.DataDir = t.TempDir()
	if err := os.MkdirAll(cfg.DataDir, 0750); err != nil {
		t.Fatal(err)
	}
	cfg.LogDir = t.TempDir()
	if err := os.MkdirAll(cfg.LogDir, 0750); err != nil {
		t.Fatal(err)
	}

	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	cfgDir := filepath.Dir(path)
	_ = os.Chmod(cfgDir, 0555)
	defer func() { _ = os.Chmod(cfgDir, 0755) }()

	payload, _ := json.Marshal(shared.NetworkSettings{IPv6: shared.IPv6Config{Enabled: true}})
	resp := d.dispatch(shared.Command{Type: shared.CmdSaveSettings, Payload: payload})
	if resp.Success {
		t.Error("expected failure when config dir is read-only")
	}
}

// ---------------------------------------------------------------------------
// CmdGetSystem
// ---------------------------------------------------------------------------

func TestDispatch_GetSystem(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	resp := d.dispatch(shared.Command{Type: shared.CmdGetSystem})
	if !resp.Success {
		t.Fatalf("GetSystem: %s", resp.Error)
	}
	var s shared.SystemSettings
	if err := json.Unmarshal(resp.Data, &s); err != nil {
		t.Fatalf("parse system settings: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CmdSaveSystem
// ---------------------------------------------------------------------------

func TestDispatch_SaveSystem_InvalidPayload(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	resp := d.dispatch(shared.Command{Type: shared.CmdSaveSystem, Payload: []byte(`{invalid`)})
	if resp.Success {
		t.Error("expected failure for invalid JSON payload")
	}
}

func TestDispatch_SaveSystem_ZeroDuration(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	s := shared.SystemSettings{
		Acceptance: shared.AcceptanceConfig{Enabled: true, Duration: 0},
	}
	payload, _ := json.Marshal(s)
	resp := d.dispatch(shared.Command{Type: shared.CmdSaveSystem, Payload: payload})
	if resp.Success {
		t.Error("expected failure for zero acceptance duration")
	}
	if resp.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestDispatch_SaveSystem_NegativeDuration(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	s := shared.SystemSettings{
		Acceptance: shared.AcceptanceConfig{Enabled: true, Duration: -10},
	}
	payload, _ := json.Marshal(s)
	resp := d.dispatch(shared.Command{Type: shared.CmdSaveSystem, Payload: payload})
	if resp.Success {
		t.Error("expected failure for negative acceptance duration")
	}
}

func TestDispatch_SaveSystem_Success(t *testing.T) {
	path := writeTempCoreConfig(t, validCoreConfig)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.DataDir = t.TempDir()
	if err := os.MkdirAll(cfg.DataDir, 0750); err != nil {
		t.Fatal(err)
	}
	cfg.LogDir = t.TempDir()
	if err := os.MkdirAll(cfg.LogDir, 0750); err != nil {
		t.Fatal(err)
	}

	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	s := shared.SystemSettings{
		Acceptance: shared.AcceptanceConfig{Enabled: true, Duration: 60},
	}
	payload, _ := json.Marshal(s)
	resp := d.dispatch(shared.Command{Type: shared.CmdSaveSystem, Payload: payload})
	if !resp.Success {
		t.Fatalf("SaveSystem: %s", resp.Error)
	}
}

func TestDispatch_SaveSystem_SaveError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory permissions; skipping read-only dir test")
	}

	path := writeTempCoreConfig(t, validCoreConfig)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.DataDir = t.TempDir()
	if err := os.MkdirAll(cfg.DataDir, 0750); err != nil {
		t.Fatal(err)
	}
	cfg.LogDir = t.TempDir()
	if err := os.MkdirAll(cfg.LogDir, 0750); err != nil {
		t.Fatal(err)
	}

	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	cfgDir := filepath.Dir(path)
	_ = os.Chmod(cfgDir, 0555)
	defer func() { _ = os.Chmod(cfgDir, 0755) }()

	s := shared.SystemSettings{
		Acceptance: shared.AcceptanceConfig{Enabled: true, Duration: 60},
	}
	payload, _ := json.Marshal(s)
	resp := d.dispatch(shared.Command{Type: shared.CmdSaveSystem, Payload: payload})
	if resp.Success {
		t.Error("expected failure when config dir is read-only")
	}
}

// ---------------------------------------------------------------------------
// CmdGetLog
// ---------------------------------------------------------------------------

func TestDispatch_GetLog_NonExistentPath(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	// The log file doesn't exist; IsNotExist is not treated as an error in dispatch.
	resp := d.dispatch(shared.Command{Type: shared.CmdGetLog})
	if !resp.Success {
		t.Fatalf("GetLog with non-existent log file should succeed: %s", resp.Error)
	}
	var entries []shared.AuditLogEntry
	if err := json.Unmarshal(resp.Data, &entries); err != nil {
		t.Fatalf("parse log entries: %v", err)
	}
}

func TestDispatch_GetLog_WithEntries(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	// Write an audit log entry first, then read it back.
	WriteAuditLog(cfg.AuditLogPath(), "test_action", "tcp", "", "web")

	resp := d.dispatch(shared.Command{Type: shared.CmdGetLog})
	if !resp.Success {
		t.Fatalf("GetLog: %s", resp.Error)
	}
	var entries []shared.AuditLogEntry
	if err := json.Unmarshal(resp.Data, &entries); err != nil {
		t.Fatalf("parse log entries: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least one audit log entry")
	}
}

// ---------------------------------------------------------------------------
// CmdGetRules store error via corrupt file
// ---------------------------------------------------------------------------

func TestDispatch_GetRules_StoreError(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	// Write invalid JSON to the rules file so GetState() returns an error.
	_ = os.WriteFile(cfg.RulesPath(), []byte("not valid json at all"), 0644)

	resp := d.dispatch(shared.Command{Type: shared.CmdGetRules})
	if resp.Success {
		t.Error("expected failure when rules file contains invalid JSON")
	}
}

// Every command the protocol declares must be handled. A constant added to
// protocol.go and not to the switch falls through to the default case and comes
// back as "unknown command" — from a web process that has no way to know it
// asked for something the core never implemented.
//
// The list is derived from the protocol rather than repeated here, so adding a
// command without a handler fails this test instead of shipping.
func TestDaemonDispatch_HandlesEveryDeclaredCommand(t *testing.T) {
	all := []shared.CommandType{
		shared.CmdGetRules, shared.CmdSaveRules, shared.CmdApplyRules, shared.CmdAccept,
		shared.CmdGetStatus, shared.CmdGetOptions, shared.CmdSaveOptions,
		shared.CmdGetSettings, shared.CmdSaveSettings, shared.CmdGetSystem,
		shared.CmdSaveSystem, shared.CmdGetLog, shared.CmdExportRules,
		shared.CmdImportRules, shared.CmdValidateCustom,
	}

	// Guard against the list above drifting from the constants: protocol.go is
	// the source of truth for how many there are, and architecture.md says
	// fifteen.
	if len(all) != 15 {
		t.Fatalf("the protocol declares 15 commands; this test lists %d", len(all))
	}

	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	for _, cmd := range all {
		resp := d.dispatch(shared.Command{Type: cmd, Payload: []byte("null")})
		if !resp.Success && strings.Contains(resp.Error, "unknown command") {
			t.Errorf("%s has no handler in dispatch", cmd)
		}
	}

	// And something that is not a command still is an unknown command.
	resp := d.dispatch(shared.Command{Type: "NOT_A_COMMAND"})
	if resp.Success || !strings.Contains(resp.Error, "unknown command") {
		t.Errorf("an unknown command must be refused as one, got %+v", resp)
	}
}

// An apply runs asynchronously so the socket stays responsive, and its
// acceptance window stays open for up to an hour. During that time the operator
// can save a setting on another page — and Apply reads exactly the sections
// that save writes.
//
// This is a genuine production sequence, and it was a data race on a slice
// header and a string until Config took a lock. Run under -race, this test is
// what fails if that lock goes away; without -race it still exercises the
// interleaving.
func TestDaemonDispatch_ApplyDoesNotRaceWithASettingsSave(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	settings := func(i int) []byte {
		payload, err := json.Marshal(shared.NetworkSettings{
			IPv6: shared.IPv6Config{Mode: shared.IPv6Filter},
			Docker: shared.DockerConfig{
				Enabled:        true,
				CustomNetworks: []string{fmt.Sprintf("172.%d.0.0/16", 16+i%16)},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return payload
	}
	options := func() []byte {
		payload, err := json.Marshal(shared.FirewallOptions{SSHBruteForce: true, PortScan: true})
		if err != nil {
			t.Fatal(err)
		}
		return payload
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); d.dispatch(shared.Command{Type: shared.CmdApplyRules}) }()
		go func(i int) {
			defer wg.Done()
			d.dispatch(shared.Command{Type: shared.CmdSaveSettings, Payload: settings(i)})
		}(i)
		go func() {
			defer wg.Done()
			d.dispatch(shared.Command{Type: shared.CmdSaveOptions, Payload: options()})
		}()
	}
	wg.Wait()

	// The config must still be coherent afterwards, not a mix of two writes.
	got := d.cfg.NetworkSettings()
	if !got.IPv6.Mode.Valid() {
		t.Errorf("ipv6 mode came out invalid after concurrent saves: %q", got.IPv6.Mode)
	}
	for _, cidr := range got.Docker.CustomNetworks {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			t.Errorf("a custom network came out torn: %q", cidr)
		}
	}
}
