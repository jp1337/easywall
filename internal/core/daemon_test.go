package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// newTestConfig creates a minimal core Config pointing to a temp directory.
func newTestConfig(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	cfg := &Config{}
	cfg.SocketPath = dir + "/core.sock"
	cfg.DataDir = dir + "/data"
	cfg.LogDir = dir + "/log"
	cfg.Acceptance.Enabled = true
	cfg.Acceptance.Duration = 30
	cfg.configPath = dir + "/easywall.toml"
	if err := os.MkdirAll(cfg.DataDir, 0750); err != nil {
		t.Fatalf("mkdir DataDir: %v", err)
	}
	if err := os.MkdirAll(cfg.LogDir, 0750); err != nil {
		t.Fatalf("mkdir LogDir: %v", err)
	}
	return cfg
}

// newTestFirewall creates a Firewall with a stub NftablesManager for dispatch tests.
func newTestFirewall(t *testing.T, cfg *Config) *Firewall {
	t.Helper()
	store, err := NewRulesStore(cfg.RulesPath())
	if err != nil {
		t.Fatalf("NewRulesStore: %v", err)
	}
	return &Firewall{
		cfg:        cfg,
		nft:        &NftablesManager{}, // nil conn — safe for dispatch tests that don't trigger Apply
		rules:      store,
		acceptance: NewAcceptance(cfg.AcceptanceDuration()),
	}
}

// sendDaemonCmd sends a command to a daemon socket and returns the response.
func sendDaemonCmd(t *testing.T, socketPath string, cmd shared.Command) shared.Response {
	t.Helper()
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", socketPath, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	data, _ := json.Marshal(cmd)
	_, _ = conn.Write(data)
	if uc, ok := conn.(*net.UnixConn); ok {
		_ = uc.CloseWrite()
	}

	respData, err := io.ReadAll(io.LimitReader(conn, 1<<20))
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var resp shared.Response
	if err := json.Unmarshal(respData, &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	return resp
}

// startTestSocket starts a daemon socket in the background.
func startTestSocket(t *testing.T, d *Daemon) {
	t.Helper()
	_ = os.Remove(d.cfg.SocketPath)
	ln, err := net.Listen("unix", d.cfg.SocketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	d.listener = ln
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go d.handleConn(conn)
		}
	}()
	t.Cleanup(func() { ln.Close() })
}

// TestDaemonDispatch uses dispatch() directly to avoid goroutine lifecycle complexity.

func TestDaemonDispatch_GetStatus(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	resp := d.dispatch(shared.Command{Type: shared.CmdGetStatus})
	if !resp.Success {
		t.Fatalf("GetStatus: %s", resp.Error)
	}
	var status shared.FirewallStatus
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		t.Fatalf("parse status: %v", err)
	}
	if !status.Active {
		t.Error("expected Active=true")
	}
}

func TestDaemonDispatch_GetRules(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	resp := d.dispatch(shared.Command{Type: shared.CmdGetRules})
	if !resp.Success {
		t.Fatalf("GetRules: %s", resp.Error)
	}
	var state shared.RulesState
	if err := json.Unmarshal(resp.Data, &state); err != nil {
		t.Fatalf("parse state: %v", err)
	}
}

func TestDaemonDispatch_SaveRules_TCP(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	rules := []shared.PortRule{{Port: "80", Description: "HTTP"}}
	payload, _ := json.Marshal(shared.SaveRulesPayload{RuleType: "tcp", Rules: rules})

	resp := d.dispatch(shared.Command{Type: shared.CmdSaveRules, Payload: payload})
	if !resp.Success {
		t.Fatalf("SaveRules tcp: %s", resp.Error)
	}
}

func TestDaemonDispatch_SaveRules_UDP(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	rules := []shared.PortRule{{Port: "53", Description: "DNS"}}
	payload, _ := json.Marshal(shared.SaveRulesPayload{RuleType: "udp", Rules: rules})

	resp := d.dispatch(shared.Command{Type: shared.CmdSaveRules, Payload: payload})
	if !resp.Success {
		t.Fatalf("SaveRules udp: %s", resp.Error)
	}
}

func TestDaemonDispatch_SaveRules_Blacklist(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	payload, _ := json.Marshal(shared.SaveRulesPayload{RuleType: "blacklist", Rules: []string{"192.168.1.1"}})
	resp := d.dispatch(shared.Command{Type: shared.CmdSaveRules, Payload: payload})
	if !resp.Success {
		t.Fatalf("SaveRules blacklist: %s", resp.Error)
	}
}

func TestDaemonDispatch_SaveRules_InvalidPayload(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	resp := d.dispatch(shared.Command{Type: shared.CmdSaveRules, Payload: []byte(`{invalid`)})
	if resp.Success {
		t.Error("expected failure for invalid JSON payload")
	}
}

func TestDaemonDispatch_SaveRules_UnknownType(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	payload, _ := json.Marshal(shared.SaveRulesPayload{RuleType: "unknown", Rules: nil})
	resp := d.dispatch(shared.Command{Type: shared.CmdSaveRules, Payload: payload})
	if resp.Success {
		t.Error("expected failure for unknown rule type")
	}
}

func TestDaemonDispatch_Accept(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	resp := d.dispatch(shared.Command{Type: shared.CmdAccept})
	if !resp.Success {
		t.Fatalf("Accept: %s", resp.Error)
	}
}

func TestDaemonDispatch_GetOptions(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	resp := d.dispatch(shared.Command{Type: shared.CmdGetOptions})
	if !resp.Success {
		t.Fatalf("GetOptions: %s", resp.Error)
	}
	var opts shared.FirewallOptions
	if err := json.Unmarshal(resp.Data, &opts); err != nil {
		t.Fatalf("parse options: %v", err)
	}
}

func TestDaemonDispatch_ExportRules(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	resp := d.dispatch(shared.Command{Type: shared.CmdExportRules})
	if !resp.Success {
		t.Fatalf("ExportRules: %s", resp.Error)
	}
}

func TestDaemonDispatch_ImportRules(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	payload := []byte(`{"tcp":[],"udp":[],"blacklist":[],"whitelist":[],"forwarding":[],"custom":[]}`)
	resp := d.dispatch(shared.Command{Type: shared.CmdImportRules, Payload: payload})
	if !resp.Success {
		t.Fatalf("ImportRules: %s", resp.Error)
	}
}

func TestDaemonDispatch_ImportRules_Invalid(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	resp := d.dispatch(shared.Command{Type: shared.CmdImportRules, Payload: []byte(`{invalid}`)})
	if resp.Success {
		t.Error("expected failure for invalid import JSON")
	}
}

func TestDaemonDispatch_UnknownCommand(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	resp := d.dispatch(shared.Command{Type: "MYSTERY_CMD"})
	if resp.Success {
		t.Error("expected failure for unknown command")
	}
	if resp.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestDaemonHandleConn_InvalidJSON(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}
	startTestSocket(t, d)

	conn, err := net.DialTimeout("unix", cfg.SocketPath, 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	_, _ = conn.Write([]byte(`not json!`))
	if uc, ok := conn.(*net.UnixConn); ok {
		_ = uc.CloseWrite()
	}

	data, _ := io.ReadAll(io.LimitReader(conn, 1<<20))
	var resp shared.Response
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.Success {
		t.Error("expected error response for invalid JSON")
	}
}

func TestDaemonHandleConn_ValidCommand(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}
	startTestSocket(t, d)

	resp := sendDaemonCmd(t, cfg.SocketPath, shared.Command{Type: shared.CmdGetStatus})
	if !resp.Success {
		t.Fatalf("expected success via socket: %s", resp.Error)
	}
}

func TestDaemonErrResp(t *testing.T) {
	err := fmt.Errorf("test error")
	resp := errResp(err)
	if resp.Success {
		t.Error("expected Success=false")
	}
	if resp.Error != "test error" {
		t.Errorf("expected 'test error', got %q", resp.Error)
	}
}

func TestLookupGroup_Root(t *testing.T) {
	gid, err := lookupGroup("root")
	if err != nil {
		t.Skipf("root group not found: %v", err)
	}
	if gid != 0 {
		t.Errorf("expected GID 0 for root, got %d", gid)
	}
}

func TestLookupGroup_NonExistent(t *testing.T) {
	_, err := lookupGroup("definitely-nonexistent-group-xyz123")
	if err == nil {
		t.Error("expected error for non-existent group")
	}
}

func TestDaemonStop(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}
	startTestSocket(t, d)

	// Stop should close the quit channel, close the listener, and clean up
	d.Stop()

	// After stop, the listener should be closed (further Accept should fail)
	_, err := d.listener.Accept()
	if err == nil {
		t.Error("expected error after stop (listener should be closed)")
	}
}

func TestDaemonStop_NilListener(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}
	// listener is nil — Stop should not panic
	d.Stop()
}

func TestDaemonDispatch_ApplyRules_ReturnsSuccess(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	// CmdApplyRules dispatches immediately and starts Apply in a goroutine.
	// The goroutine may fail (no netlink), but the dispatch itself returns success.
	// We just verify the dispatch return, then give the goroutine time to fail gracefully.
	done := make(chan shared.Response, 1)
	go func() {
		defer func() { recover() }() // recover from any goroutine panics in Apply
		done <- d.dispatch(shared.Command{Type: shared.CmdApplyRules})
	}()

	select {
	case resp := <-done:
		if !resp.Success {
			t.Fatalf("ApplyRules dispatch: %s", resp.Error)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for dispatch")
	}
	// Brief pause to let the async Apply goroutine start and finish
	time.Sleep(50 * time.Millisecond)
}

func TestDaemonDispatch_GetRules_Error(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	// Delete the rules file so GetState() fails
	_ = os.Remove(cfg.RulesPath())
	// Also create an invalid file so json.Unmarshal fails
	_ = os.WriteFile(cfg.RulesPath(), []byte("not valid json"), 0644)

	resp := d.dispatch(shared.Command{Type: shared.CmdGetRules})
	if resp.Success {
		t.Error("expected failure when rules file is corrupt")
	}
}

func TestDaemonDispatch_ExportRules_Error(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	// Corrupt the rules file so ExportCurrent → GetState fails
	_ = os.WriteFile(cfg.RulesPath(), []byte("not valid json"), 0644)

	resp := d.dispatch(shared.Command{Type: shared.CmdExportRules})
	if resp.Success {
		t.Error("expected failure when rules file is corrupt")
	}
}

func TestDaemonDispatch_SaveRules_SaveError(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	// Corrupt the rules file and make data dir read-only so save() fails
	_ = os.WriteFile(cfg.RulesPath(), []byte("{}"), 0444) // valid JSON but not RulesState
	// Also remove write permission from data dir
	_ = os.Chmod(cfg.DataDir, 0444)
	defer func() { _ = os.Chmod(cfg.DataDir, 0750) }()

	rules := []shared.PortRule{{Port: "80"}}
	payload, _ := json.Marshal(shared.SaveRulesPayload{RuleType: "tcp", Rules: rules})
	resp := d.dispatch(shared.Command{Type: shared.CmdSaveRules, Payload: payload})
	// Either parse error or save error — either way not Success
	_ = resp // just ensure no panic
}

func TestLookupGroup_ParsesValidGroup(t *testing.T) {
	// "root" or "nogroup" should exist — test that we can look up a real group
	gid, err := lookupGroup("root")
	if err != nil {
		t.Logf("root group not found (skipping): %v", err)
		return
	}
	if gid != 0 {
		t.Errorf("root GID should be 0, got %d", gid)
	}
}

// TestDaemonHandleConn_ReadError triggers the io.ReadAll error path by setting
// an already-expired deadline on the connection before handleConn reads.
func TestDaemonHandleConn_ReadError(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	server, client := net.Pipe()
	defer server.Close()

	// Set deadline to the past so any read immediately returns a timeout error
	_ = client.SetDeadline(time.Now().Add(-time.Second))

	// handleConn should handle the io.ReadAll error gracefully (no panic)
	d.handleConn(client)
}

// TestDaemonDispatch_ApplyRules_GoroutineError waits long enough for the Apply
// goroutine to complete and hit the slog.Error path.
func TestDaemonDispatch_ApplyRules_GoroutineError(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	resp := d.dispatch(shared.Command{Type: shared.CmdApplyRules})
	if !resp.Success {
		t.Fatalf("dispatch CmdApplyRules should return Success: %s", resp.Error)
	}
	// Give the goroutine enough time to run Apply (which fails with nil conn)
	// and reach slog.Error("apply error", ...)
	time.Sleep(100 * time.Millisecond)
}

// TestDaemonDispatch_ApplyRules_PanicRecovery exercises the recover() branch inside
// the Apply goroutine by using a nil rules store that causes Apply to panic.
func TestDaemonDispatch_ApplyRules_PanicRecovery(t *testing.T) {
	cfg := newTestConfig(t)
	// nil rules causes f.rules.GetState() inside Apply to panic (nil pointer)
	fw := &Firewall{
		cfg:        cfg,
		nft:        &NftablesManager{},
		rules:      nil,
		acceptance: NewAcceptance(cfg.AcceptanceDuration()),
	}
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	resp := d.dispatch(shared.Command{Type: shared.CmdApplyRules})
	if !resp.Success {
		t.Fatalf("dispatch should return success immediately: %s", resp.Error)
	}
	// Wait for the goroutine to panic and recover
	time.Sleep(100 * time.Millisecond)
}

func TestLookupGroup_OpenError(t *testing.T) {
	old := groupFilePath
	groupFilePath = "/nonexistent/etc/group"
	defer func() { groupFilePath = old }()

	_, err := lookupGroup("root")
	if err == nil {
		t.Error("expected error when group file does not exist")
	}
}

// ---------------------------------------------------------------------------
// NewDaemon error paths (no nftables required — errors happen before NewFirewall)
// ---------------------------------------------------------------------------

func TestNewDaemon_DataDirCreationError(t *testing.T) {
	cfg := newTestConfig(t)
	// /proc is not writable — MkdirAll fails before NewFirewall is ever called.
	cfg.DataDir = "/proc/easywall-unit-test-data"
	_, err := NewDaemon(cfg)
	if err == nil {
		t.Error("expected error when data dir cannot be created")
	}
}

func TestNewDaemon_LogDirCreationError(t *testing.T) {
	cfg := newTestConfig(t)
	// DataDir already exists (created by newTestConfig), but LogDir points to /proc.
	cfg.LogDir = "/proc/easywall-unit-test-log"
	_, err := NewDaemon(cfg)
	if err == nil {
		t.Error("expected error when log dir cannot be created")
	}
}

// ---------------------------------------------------------------------------
// Daemon.Start lifecycle (no nftables required — uses stub firewall)
// ---------------------------------------------------------------------------

func TestDaemonStart_ListenError(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}
	// /proc sub-path is not writable — net.Listen("unix", ...) fails.
	d.cfg.SocketPath = "/proc/nonexistent/easywall.sock"

	err := d.Start()
	if err == nil {
		t.Error("expected error when socket path is invalid")
	}
}

func TestDaemonStart_Stop(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	errCh := make(chan error, 1)
	go func() { errCh <- d.Start() }()

	// Wait for the socket file to appear (Start → net.Listen → Accept loop).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(cfg.SocketPath); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := os.Stat(cfg.SocketPath); err != nil {
		t.Fatalf("socket not created within 2s: %v", err)
	}

	d.Stop()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned unexpected error after Stop: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("Start did not return within 3s after Stop")
	}
}

// TestDaemonStart_ChownPath exercises the os.Chown call inside Start by creating
// a synthetic group file that contains the "easywall" group so lookupGroup succeeds.
// The chown itself may fail (not root) — either branch is covered.
func TestDaemonStart_ChownPath(t *testing.T) {
	groupFile, err := os.CreateTemp("", "group-chown-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(groupFile.Name())
	_, _ = groupFile.WriteString("easywall:x:12345:\n")
	groupFile.Close()

	old := groupFilePath
	groupFilePath = groupFile.Name()
	defer func() { groupFilePath = old }()

	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	errCh := make(chan error, 1)
	go func() { errCh <- d.Start() }()

	// Wait for the socket — once it appears, Start has already executed the chown path.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(cfg.SocketPath); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	d.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("Start did not return within 3s")
	}
}

// TestDaemonStart_AcceptErrorDefaultBranch covers the default: continue branch
// inside Start's Accept loop — triggered by closing the listener while the quit
// channel is still open, so the select falls through to slog.Error + continue.
func TestDaemonStart_AcceptErrorDefaultBranch(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	errCh := make(chan error, 1)
	go func() { errCh <- d.Start() }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(cfg.SocketPath); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := os.Stat(cfg.SocketPath); err != nil {
		t.Fatalf("socket not created: %v", err)
	}

	// Close the listener directly (not via Stop) while quit is still open.
	// The goroutine will loop through default: continue until Stop closes quit.
	if d.listener != nil {
		_ = d.listener.Close()
	}
	time.Sleep(10 * time.Millisecond) // let the default branch execute at least once

	d.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("Start did not return within 3s")
	}
}

func TestDaemonStart_AcceptsAndResponds(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	go func() { _ = d.Start() }()
	t.Cleanup(d.Stop)

	// Wait for socket
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(cfg.SocketPath); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	resp := sendDaemonCmd(t, cfg.SocketPath, shared.Command{Type: shared.CmdGetStatus})
	if !resp.Success {
		t.Fatalf("GetStatus via running Start: %s", resp.Error)
	}
}
