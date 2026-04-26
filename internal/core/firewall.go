package core

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// Firewall orchestrates the full rule-application lifecycle:
// backup → apply → acceptance window → commit or rollback.
type Firewall struct {
	cfg        *Config
	nft        *NftablesManager
	rules      *RulesStore
	acceptance *Acceptance
	mu         sync.Mutex // prevents concurrent Apply calls
	lastApply  time.Time
}

// NewFirewall creates a Firewall with all sub-components initialised.
func NewFirewall(cfg *Config) (*Firewall, error) {
	nft, err := NewNftablesManager()
	if err != nil {
		return nil, fmt.Errorf("init nftables: %w", err)
	}

	store, err := NewRulesStore(cfg.RulesPath())
	if err != nil {
		return nil, fmt.Errorf("init rules store: %w", err)
	}

	return &Firewall{
		cfg:        cfg,
		nft:        nft,
		rules:      store,
		acceptance: NewAcceptance(cfg.AcceptanceDuration()),
	}, nil
}

// Apply starts a full rule-application cycle.
// It blocks until the acceptance window completes (accepted or timed out).
// Thread-safe: concurrent Apply calls are serialised.
func (f *Firewall) Apply(user string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	slog.Info("starting rule apply", "user", user)

	state, err := f.rules.GetState()
	if err != nil {
		return fmt.Errorf("get rules: %w", err)
	}

	// 1. Backup current rules (for rollback)
	if err := f.rules.BackupCurrent(); err != nil {
		return fmt.Errorf("backup rules: %w", err)
	}

	// 2. Take nftables snapshot for emergency recovery
	snap, err := f.nft.Snapshot()
	if err != nil {
		slog.Warn("could not take nftables snapshot", "error", err)
	} else if f.cfg.LogDir != "" {
		_ = SaveSnapshot(f.cfg.LogDir, snap)
	}

	// 3. Promote staged → current in state
	if err := f.rules.PromoteStaged(); err != nil {
		return fmt.Errorf("promote staged rules: %w", err)
	}

	updatedState, err := f.rules.GetState()
	if err != nil {
		return fmt.Errorf("re-read rules after promote: %w", err)
	}

	// 4. Apply new rules to kernel
	if err := f.nft.Apply(updatedState, f.cfg.Firewall, f.cfg.IPv6, f.cfg.Docker); err != nil {
		// Rule application failed — roll back immediately without waiting
		_ = f.rules.Rollback()
		_ = f.nft.Apply(state, f.cfg.Firewall, f.cfg.IPv6, f.cfg.Docker)
		WriteAuditLog(f.cfg.AuditLogPath(), "apply_failed", "all", err.Error(), user)
		return fmt.Errorf("apply nftables rules: %w", err)
	}

	WriteAuditLog(f.cfg.AuditLogPath(), "apply_started", "all", "", user)

	// 5. Start acceptance window
	if err := f.acceptance.Start(); err != nil {
		return err
	}

	accepted := f.acceptance.Wait()

	if !accepted {
		slog.Warn("acceptance timeout — rolling back")
		if err := f.rules.Rollback(); err != nil {
			slog.Error("rollback rules file failed", "error", err)
		}
		if err := f.nft.Apply(state, f.cfg.Firewall, f.cfg.IPv6, f.cfg.Docker); err != nil {
			slog.Error("rollback nftables failed", "error", err)
		}
		WriteAuditLog(f.cfg.AuditLogPath(), "apply_rolledback", "all", "timeout", user)
		return nil
	}

	f.lastApply = time.Now()
	WriteAuditLog(f.cfg.AuditLogPath(), "apply_accepted", "all", "", user)
	slog.Info("rules applied and accepted", "user", user)
	return nil
}

// Accept signals that the admin confirmed the new rules work correctly.
func (f *Firewall) Accept() {
	f.acceptance.Accept()
}

// Status returns the current firewall status for dashboard display.
func (f *Firewall) Status() shared.FirewallStatus {
	pending, _ := f.rules.HasPendingChanges()
	lastApply := ""
	if !f.lastApply.IsZero() {
		lastApply = f.lastApply.UTC().Format(time.RFC3339)
	}
	return shared.FirewallStatus{
		Active:     true, // if daemon is running, firewall is active
		Acceptance: f.acceptance.Status(),
		HasPending: pending,
		LastApply:  lastApply,
	}
}

// RulesStore returns the underlying rules store for direct rule manipulation.
func (f *Firewall) RulesStore() *RulesStore {
	return f.rules
}

// Options returns the current firewall options.
func (f *Firewall) Options() shared.FirewallOptions {
	return f.cfg.Firewall
}
