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
	"time"

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
// CmdApplyRules
// ---------------------------------------------------------------------------

// APPLY_RULES has to refuse synchronously, the same way ErrApplyInProgress
// does, or the refusal never reaches the caller at all: the goroutine that
// runs apply() only logs its error, and the immediate response was already
// {"status":"started"} by the time apply() ran and found the marker. A
// browser tab clicking Apply after `panic` ran at the console needs to be
// told so, not left to infer it from a status page that never changes.
func TestDispatch_ApplyRules_RefusedWhilePanicIsEngaged(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	if err := EngagePanic(cfg.PanicMarkerPath()); err != nil {
		t.Fatalf("EngagePanic: %v", err)
	}

	resp := d.dispatch(shared.Command{Type: shared.CmdApplyRules})
	if resp.Success {
		t.Fatalf("APPLY_RULES succeeded while panic mode was engaged: %+v", resp)
	}
	if resp.Error != shared.ErrPanicEngagedText {
		t.Errorf("Response.Error = %q, want %q — the web process matches on this exact "+
			"string to explain the refusal instead of reporting a generic failure",
			resp.Error, shared.ErrPanicEngagedText)
	}

	// No goroutine was started: the slot was never claimed, so beginApply
	// still succeeds, and nothing is tracked in d.wg for Stop to wait on.
	if !fw.beginApply() {
		t.Error("the apply slot was claimed even though the request was refused before beginApply")
	}
	fw.endApply()

	done := make(chan struct{})
	go func() {
		d.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop did not return promptly; a refused APPLY_RULES left something running")
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
	// Derive the list from the protocol's published list, so adding a command
	// and forgetting a handler fails here rather than shipping. The list is in
	// shared.AllCommandTypes; this test does not maintain its own.
	all := shared.AllCommandTypes

	// Guard against an empty or truncated list: an accident in AllCommandTypes
	// that makes this test vacuous must be caught. This is not a great check —
	// it only catches gross errors — but even that is better than nothing.
	if len(all) < 15 {
		t.Fatalf("AllCommandTypes has %d entries; the protocol should have at least 15 commands", len(all))
	}

	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	// APPLY_RULES hands the work to a goroutine; Stop waits for it, so the test
	// cannot return while it is still writing into t.TempDir().
	defer d.Stop()

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

	// The apply is called directly rather than through CmdApplyRules, which
	// hands it to a goroutine this test has no handle on: those outlive the
	// test, and one of them writing its snapshot into t.TempDir() after the
	// cleanup has run makes the suite flaky for reasons that have nothing to do
	// with what is being tested. The config reads that race happen in Apply
	// either way, before it ever reaches netlink.
	cfg.Acceptance.Enabled = false // no window to wait out

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			_ = d.firewall.Apply("test")
		}()
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

func TestDispatch_PanicEngagesAndResumeClears(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}
	defer d.Stop()

	// The teardown fails on the nil netlink connection, so the reply is a
	// failure — and the marker is still written, which is the behaviour that
	// keeps a reboot from undoing the operator's decision.
	resp := d.dispatch(shared.Command{Type: shared.CmdPanic})
	if resp.Success {
		t.Error("with no netlink connection PANIC must report the teardown failure")
	}
	if !PanicEngaged(cfg.PanicMarkerPath()) {
		t.Error("PANIC must leave the marker behind even when the teardown failed")
	}

	if got := d.dispatch(shared.Command{Type: shared.CmdGetStatus}); !got.Success {
		t.Fatalf("GET_STATUS failed: %s", got.Error)
	} else {
		var status shared.FirewallStatus
		if err := json.Unmarshal(got.Data, &status); err != nil {
			t.Fatalf("parse status: %v", err)
		}
		if !status.Panic {
			t.Error("GET_STATUS must report panic mode")
		}
	}

	// Resume clears the marker and then tries to restore the stored rules. With
	// the nil netlink connection, the restore fails, so the response reports
	// failure — but the marker is still cleared, the symmetric behaviour to
	// PANIC: a marker left behind survives a reboot.
	resp = d.dispatch(shared.Command{Type: shared.CmdResume})
	if resp.Success {
		t.Error("with no netlink connection RESUME must report the restore failure")
	}
	if PanicEngaged(cfg.PanicMarkerPath()) {
		t.Error("RESUME must clear the marker even when the restore failed")
	}
}
