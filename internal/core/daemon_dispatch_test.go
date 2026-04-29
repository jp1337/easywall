package core

import (
	"encoding/json"
	"os"
	"path/filepath"
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
