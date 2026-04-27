package core

import (
	"os"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

func TestFirewallStatus_WithLastApply(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	fw.lastApply = time.Now()

	status := fw.Status()
	if status.LastApply == "" {
		t.Error("expected non-empty LastApply when lastApply is set")
	}
}

func TestFirewallStatus_WithoutLastApply(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	status := fw.Status()
	if status.LastApply != "" {
		t.Errorf("expected empty LastApply, got: %s", status.LastApply)
	}
	if !status.Active {
		t.Error("expected Active=true")
	}
}

func TestFirewallStatus_WithPendingChanges(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	// Stage a TCP rule to create a pending change
	_ = fw.rules.SaveStaged("tcp", []shared.PortRule{{Port: "8080"}})

	status := fw.Status()
	if !status.HasPending {
		t.Error("expected HasPending=true after staging rules")
	}
}

func TestFirewallStatus_AcceptanceState(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	// Initial state should be idle
	status := fw.Status()
	if status.Acceptance != shared.AcceptanceIdle {
		t.Errorf("expected idle acceptance, got %s", status.Acceptance)
	}
}

func TestFirewallRulesStore(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	if fw.RulesStore() == nil {
		t.Error("expected non-nil rules store")
	}
}

func TestFirewallOptions(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Firewall.SSHBruteForce = true
	fw := newTestFirewall(t, cfg)

	opts := fw.Options()
	if !opts.SSHBruteForce {
		t.Error("expected SSHBruteForce=true")
	}
}

func TestFirewallAccept_IdleState(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	// Accept when idle — should be a no-op
	fw.Accept()
	status := fw.Status()
	if status.Acceptance != shared.AcceptanceIdle {
		t.Errorf("accept when idle should leave status idle, got %s", status.Acceptance)
	}
}

// TestFirewallApply_GetStateError tests Apply's early exit when GetState fails.
func TestFirewallApply_GetStateError(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	// Corrupt the rules file so GetState fails
	_ = os.WriteFile(cfg.RulesPath(), []byte("not valid json"), 0644)

	err := fw.Apply("test")
	if err == nil {
		t.Error("expected error when rules file is corrupt")
	}
}

// TestFirewallApply_BackupError tests Apply's exit when BackupCurrent fails.
// This covers the "backup rules" error path in Apply.
// We use 0555 (read+execute, no write) so GetState can still read the rules file
// but os.CreateTemp inside save() fails because the dir is not writable.
func TestFirewallApply_BackupError(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	// 0555: read+execute but NOT write — GetState succeeds, save() CreateTemp fails
	_ = os.Chmod(cfg.DataDir, 0555)
	defer func() { _ = os.Chmod(cfg.DataDir, 0750) }()

	err := fw.Apply("test")
	if err == nil {
		t.Error("expected error when data dir is not writable")
	}
}

// TestFirewallApply_NilConn tests Apply when nftables conn is nil.
// With nil conn, Snapshot returns error (but Apply continues),
// then nft.Apply (via Reset) also returns error — triggering the rollback path.
// TestNewFirewall_CodePath exercises the NewFirewall function body.
// In unit test environments without nftables the function returns an error at
// NewNftablesManager; in privileged environments it returns a valid Firewall.
// Either outcome is correct — this test exists for code-path coverage.
func TestNewFirewall_CodePath(t *testing.T) {
	cfg := newTestConfig(t)
	fw, err := NewFirewall(cfg)
	if err != nil {
		return // nftables not available — expected in CI
	}
	if fw == nil {
		t.Error("expected non-nil Firewall when err is nil")
	}
}

func TestFirewallApply_NilConn(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg) // nft.conn is nil

	// Stage a rule so there's something to apply
	_ = fw.rules.SaveStaged("tcp", []shared.PortRule{{Port: "80"}})

	// Apply will fail at nft.Apply (nft.Reset → conn is nil)
	err := fw.Apply("test")
	if err == nil {
		t.Error("expected error when nftables connection is nil")
	}
}
