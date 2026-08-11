package core

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
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

	// lastApplyMu guards lastApply, deliberately separate from mu: Apply holds
	// mu for the whole acceptance window — up to an hour — and the dashboard
	// polls Status throughout exactly that window. Sharing one lock would make
	// the status page hang for the entire time it matters most.
	lastApplyMu sync.Mutex
	lastApply   time.Time
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

	f := &Firewall{
		cfg:        cfg,
		nft:        nft,
		rules:      store,
		acceptance: NewAcceptance(cfg.AcceptanceDuration()),
	}
	f.lastApply = readLastApply(cfg.LastApplyPath())
	return f, nil
}

// readLastApply loads the recorded time of the last accepted apply. A missing
// or unreadable file means "not known", which is what a fresh install is.
func readLastApply(path string) time.Time {
	data, err := os.ReadFile(path) // #nosec G304 -- path is built from the daemon's own config
	if err != nil {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		slog.Warn("ignoring unreadable last-apply marker", "path", path, "error", err)
		return time.Time{}
	}
	return t
}

// setLastApply records the time of an accepted apply, in memory and on disk.
func (f *Firewall) setLastApply(t time.Time) {
	f.lastApplyMu.Lock()
	f.lastApply = t
	f.lastApplyMu.Unlock()

	path := f.cfg.LastApplyPath()
	if err := os.WriteFile(path, []byte(t.UTC().Format(time.RFC3339)), 0640); err != nil {
		// Not fatal: the rules are applied and accepted either way. The
		// dashboard will just show "never" again after a restart.
		slog.Warn("could not record last apply time", "path", path, "error", err)
	}
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
	// One snapshot for the whole apply, so the rules that reach the kernel
	// describe a single configuration rather than whatever each field happened
	// to hold as it was read.
	if err := f.nft.Apply(updatedState, f.cfg.FirewallOptions(), f.cfg.NetworkSettings()); err != nil {
		// Rule application failed — roll back immediately without waiting
		WriteAuditLog(f.cfg.AuditLogPath(), "apply_failed", "all", err.Error(), user)
		f.rollback(state, user)
		return fmt.Errorf("apply nftables rules: %w", err)
	}

	// 5. Acceptance window, unless it has been switched off.
	//
	// acceptance.enabled was never read until 2.5.0. The system settings page
	// offers the switch and documents "Off — an apply is final. There is no
	// automatic way back", but the window ran regardless: an operator who
	// deliberately turned it off, on a machine they can physically reach, still
	// had the change rolled out from under them when the timer expired.
	if !f.cfg.SystemSettings().Acceptance.Enabled {
		WriteAuditLog(f.cfg.AuditLogPath(), "apply_started", "all",
			"acceptance window disabled — applied without confirmation", user)
		f.setLastApply(time.Now())
		WriteAuditLog(f.cfg.AuditLogPath(), "apply_accepted", "all",
			"no confirmation required", user)
		slog.Info("rules applied; acceptance window is disabled", "user", user)
		return nil
	}

	WriteAuditLog(f.cfg.AuditLogPath(), "apply_started", "all", "", user)

	if err := f.acceptance.Start(f.cfg.AcceptanceDuration()); err != nil {
		return err
	}

	accepted := f.acceptance.Wait()
	defer f.acceptance.Reset()

	if !accepted {
		slog.Warn("acceptance timeout — rolling back")
		WriteAuditLog(f.cfg.AuditLogPath(), "apply_rolledback", "all", "timeout", user)
		f.rollback(state, user)
		return nil
	}

	f.setLastApply(time.Now())
	WriteAuditLog(f.cfg.AuditLogPath(), "apply_accepted", "all", "", user)
	slog.Info("rules applied and accepted", "user", user)
	return nil
}

// rollback restores the rule set that was in force before this apply.
//
// Both callers previously discarded its errors with `_ =`, which made the worst
// outcome the quietest one: an apply that failed *and* whose rollback failed
// left the host with whatever half-state the kernel had, and the only record
// was the original error. A failed rollback gets its own audit entry, because
// it is the line an operator needs to find first.
func (f *Firewall) rollback(previous shared.RulesState, user string) {
	var failures []string

	if err := f.rules.Rollback(); err != nil {
		slog.Error("rollback rules file failed", "error", err)
		failures = append(failures, "rules file: "+err.Error())
	}
	if err := f.nft.Apply(previous, f.cfg.FirewallOptions(), f.cfg.NetworkSettings()); err != nil {
		slog.Error("rollback nftables failed", "error", err)
		failures = append(failures, "nftables: "+err.Error())
	}

	if len(failures) > 0 {
		WriteAuditLog(f.cfg.AuditLogPath(), "rollback_failed", "all",
			strings.Join(failures, "; "), user)
	}
}

// Accept signals that the admin confirmed the new rules work correctly, and
// reports whether a window was open to receive it.
func (f *Firewall) Accept() bool {
	return f.acceptance.Accept()
}

// CancelAcceptance ends an open window as not accepted, so the apply that owns
// it rolls back and returns. Used on shutdown.
func (f *Firewall) CancelAcceptance() {
	f.acceptance.Cancel()
}

// Status returns the current firewall status for dashboard display.
func (f *Firewall) Status() shared.FirewallStatus {
	pending, _ := f.rules.HasPendingChanges()

	f.lastApplyMu.Lock()
	last := f.lastApply
	f.lastApplyMu.Unlock()

	lastApply := ""
	if !last.IsZero() {
		lastApply = last.UTC().Format(time.RFC3339)
	}

	return shared.FirewallStatus{
		// Asked of the kernel, not inferred from this process being alive. The
		// dashboard renders this as "rules are live", which is a claim about
		// what the kernel holds; answering it with "the daemon is running" made
		// that sentence unverified, and green after the table had been deleted.
		Active:     f.nft.Enforcing(),
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
	return f.cfg.FirewallOptions()
}
