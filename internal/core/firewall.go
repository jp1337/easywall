package core

import (
	"errors"
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

	// applyMu guards applying, which records that a cycle is running.
	//
	// This was a plain mutex that Apply held for the whole cycle, so a second
	// apply did not fail — it *waited*, for as long as the acceptance window had
	// left. See ErrApplyInProgress for what that cost.
	applyMu  sync.Mutex
	applying bool

	// lastApplyMu guards lastApply, deliberately separate: a cycle runs for the
	// whole acceptance window — up to an hour — and the dashboard polls Status
	// throughout exactly that window. Sharing one lock would make the status
	// page hang for the entire time it matters most.
	lastApplyMu sync.Mutex
	lastApply   time.Time
}

// ErrApplyInProgress is returned when an apply is asked for while a cycle is
// already running.
//
// It used to be queued instead. Apply serialised on a mutex it holds for the
// whole acceptance window, so a second APPLY_RULES sat in that lock until the
// first window closed and then ran on its own. Two consequences, both measured
// against a real kernel:
//
//   - The queued apply promoted the staged set again the instant the first one
//     rolled back. An operator whose rules cut their connection, who waited out
//     the window to get back in, was cut off again by an apply nobody
//     re-requested. That is the product's central promise running backwards.
//   - Stop cancels the window that is open, not the ones queued behind it. Four
//     APPLY commands made shutdown wait three further full windows: 6.1 s at a
//     2 s window, and six minutes at the shipped 120 s default — past systemd's
//     90 s TimeoutStopSec, after which SIGKILL leaves the unconfirmed rules live
//     and no rollback runs at all.
//
// The web interface hides the Start button while a window is open, which is why
// this went unnoticed; a second tab, a double-click before the redirect lands,
// or the back button all still reach the endpoint, and the privileged side does
// not get to depend on the browser hiding a control.
var ErrApplyInProgress = errors.New(shared.ErrApplyInProgressText)

// ErrPanicEngaged is returned when an apply is asked for while panic mode is
// engaged.
//
// beginApply claims the slot before this is checked, so a refused apply still
// releases it immediately — nothing is left holding it open. The check has to
// live in apply, not Apply: the daemon calls beginApply() and then apply()
// directly (see dispatch's CmdApplyRules case), so a guard only on the
// exported wrapper would never run on the path a real request actually takes.
//
// This is what makes Panic's refusal to be undone by the browser. Nothing
// stops an operator's other browser tab, or a request queued before they ran
// `panic` at the console, from reaching this function afterwards; without the
// check it would re-open the table the console just closed, which is exactly
// the outcome panic mode exists to rule out.
var ErrPanicEngaged = errors.New(shared.ErrPanicEngagedText)

// beginApply claims the right to run one apply cycle, reporting false when one
// is already running. endApply releases it.
//
// A flag rather than a mutex because the answer has to be available *without*
// waiting: the caller's job is to tell the operator whether their apply
// started, and "it will start in an hour" is not one of the answers.
func (f *Firewall) beginApply() bool {
	f.applyMu.Lock()
	defer f.applyMu.Unlock()
	if f.applying {
		return false
	}
	f.applying = true
	return true
}

func (f *Firewall) endApply() {
	f.applyMu.Lock()
	f.applying = false
	f.applyMu.Unlock()
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
	// 0600 for the same reason as the audit log: only this process reads it —
	// the dashboard's "last apply" comes over the socket with the status, not
	// from this file — so the group bit gave the web process a read it never
	// takes.
	if err := os.WriteFile(path, []byte(t.UTC().Format(time.RFC3339)), 0600); err != nil {
		// Not fatal: the rules are applied and accepted either way. The
		// dashboard will just show "never" again after a restart.
		slog.Warn("could not record last apply time", "path", path, "error", err)
	}
}

// Apply starts a full rule-application cycle.
// It blocks until the acceptance window completes (accepted or timed out).
//
// A second Apply arriving while one is running is refused with
// ErrApplyInProgress rather than queued behind the open window — read the note
// on that error before changing it back.
func (f *Firewall) Apply(user string) error {
	if !f.beginApply() {
		return ErrApplyInProgress
	}
	defer f.endApply()
	return f.apply(user)
}

// apply runs the cycle. The caller must already hold the slot from beginApply,
// which is why this is separate: the daemon has to claim the slot
// *synchronously*, so it can answer the request truthfully, and only then hand
// the cycle itself to a goroutine.
func (f *Firewall) apply(user string) error {
	// The marker outranks the slot. beginApply only proves nobody else is
	// mid-cycle; it says nothing about whether the console has taken the
	// firewall down since. Checking here, inside the function every real
	// request path reaches, means a request that arrived before `panic` ran,
	// one queued in another tab, or a client that simply has not been told
	// yet all land on the same refusal rather than three different races
	// against Reset().
	if f.PanicEngaged() {
		WriteAuditLog(f.cfg.AuditLogPath(), "apply_refused_panic", "all",
			"an apply was refused because panic mode is engaged at the console", user)
		slog.Warn("apply refused: panic mode is engaged", "user", user)
		return ErrPanicEngaged
	}

	slog.Info("starting rule apply", "user", user)

	state, err := f.rules.GetState()
	if err != nil {
		// The audit log is what the operator looks at when the interface says
		// the firewall is off; a failure that only reaches the journal is invisible.
		WriteAuditLog(f.cfg.AuditLogPath(), "apply_failed", "all", err.Error(), user)
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
	// The check that makes Panic's marker authoritative rather than advisory.
	//
	// Panic does not take the apply slot — it has to be able to interrupt a
	// cycle that already holds it, from the console, which is the whole point
	// of a panic button. That leaves this as the only place left to stop an
	// apply's rollback from flushing the previous rules back into the kernel
	// on top of the teardown Panic just did: cancelling the acceptance window
	// is what *starts* the rollback (Wait() sees the cancel and returns
	// false), not what stops it. Refusing here, on the current state of the
	// marker rather than on anything decided earlier in the cycle, is also
	// what catches the slower variant: Panic landing between beginApply and
	// acceptance.Start, where CancelAcceptance has nothing pending to cancel
	// and the window still runs its full length before rollback is ever
	// called.
	if f.PanicEngaged() {
		WriteAuditLog(f.cfg.AuditLogPath(), "rollback_skipped", "all",
			"panic mode is engaged, so the previous rules were not restored", user)
		return
	}

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
