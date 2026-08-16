package core

import (
	"fmt"
	"log/slog"
)

// Why the reason a restore happened is a constant and not free text: it reaches
// the audit log, the audit log is rendered through auditActionLabels in the web
// process, and a detail string assembled at the call site is one refactor away
// from carrying something that has no translation.
const (
	// RestoreReasonBoot is the daemon starting up.
	RestoreReasonBoot = "daemon start"
	// RestoreReasonResume is an operator ending panic mode.
	RestoreReasonResume = "panic mode ended"
)

// PanicEngaged reports whether this installation is deliberately unfiltered.
func (f *Firewall) PanicEngaged() bool {
	return PanicEngaged(f.cfg.PanicMarkerPath())
}

// RestoreCurrent puts the stored Current rule set back into the kernel, with no
// acceptance window.
//
// The missing window looks like a breach of easywall's central promise and is
// not one. Current is, by definition, a rule set that has already survived an
// acceptance window: it got there through PromoteStaged, which runs only inside
// an apply, and an apply that was not confirmed rolls Current back. Restoring it
// changes nothing about the machine's intended state. Refusing to restore it is
// what changes something.
//
// A window here could not work even if it were wanted. Nobody is present at boot
// to confirm, so it would expire; the rollback would install `backup`, which
// nobody confirmed either; and there is no second window to catch that. A loop
// with no exit.
//
// Before this existed, nftables was simply empty after a reboot and the machine
// stayed unfiltered until somebody opened the interface and pressed Apply. The
// dashboard reported it correctly — Status asks the kernel — but only to
// somebody who was looking.
func (f *Firewall) RestoreCurrent(reason string) error {
	if f.PanicEngaged() {
		slog.Warn("panic mode is engaged, so the stored rules are not being restored; "+
			"this machine is deliberately unfiltered — run `easywall-core resume` to end it",
			"reason", reason)
		return nil
	}

	// The same slot an apply takes, so a restore and an apply can never both be
	// writing table inet easywall. At boot nothing competes; RESUME can arrive at
	// any time.
	if !f.beginApply() {
		return ErrApplyInProgress
	}
	defer f.endApply()

	state, err := f.rules.GetState()
	if err != nil {
		return fmt.Errorf("get rules: %w", err)
	}

	if err := f.nft.Apply(state, f.cfg.FirewallOptions(), f.cfg.NetworkSettings()); err != nil {
		// Recorded, not just returned. This is the line an operator needs when
		// the machine came up unfiltered and nobody can say why.
		WriteAuditLog(f.cfg.AuditLogPath(), "boot_enforce_failed", "all",
			fmt.Sprintf("%s: %s", reason, err.Error()), "core")
		return fmt.Errorf("restore rules: %w", err)
	}

	WriteAuditLog(f.cfg.AuditLogPath(), "boot_enforced", "all", reason, "core")
	slog.Info("the stored rules are in force again", "reason", reason)
	return nil
}
