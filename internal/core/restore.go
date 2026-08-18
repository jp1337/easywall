package core

import (
	"errors"
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
		// The audit log is what the operator looks at when the interface says
		// the firewall is off; a failure that only reaches the journal is invisible.
		WriteAuditLog(f.cfg.AuditLogPath(), "boot_enforce_failed", "all",
			fmt.Sprintf("%s: %s", reason, err.Error()), "core")
		return fmt.Errorf("get rules: %w", err)
	}

	// What this apply is about to bake in, so reconcileDockerBridges can tell
	// "no bridges yet" from "no bridges at all". Guarded, not a plain field
	// write, because reconcileDockerBridges reads and writes the same field
	// from the goroutine Daemon.Start launches. At this commit that goroutine
	// and this call never actually overlap — Start's own boot restore is a
	// plain, sequential call that finishes before the reconciler goroutine is
	// even launched, and Resume has no production caller yet, so nothing else
	// can reach RestoreCurrent while the reconciler is running. The guard is
	// deliberate hardening ahead of Resume being wired to the socket handler,
	// where RESUME can arrive at any time — not a fix for a race that exists
	// today.
	f.setBootBridges(detectDockerBridges())

	if err := f.nft.Apply(state, f.cfg.FirewallOptions(), f.cfg.NetworkSettings()); err != nil {
		// Recorded, not just returned. This is the line an operator needs when
		// the machine came up unfiltered and nobody can say why.
		WriteAuditLog(f.cfg.AuditLogPath(), "boot_enforce_failed", "all",
			fmt.Sprintf("%s: %s", reason, err.Error()), "core")
		return fmt.Errorf("restore rules: %w", err)
	}

	// The marker again, now that the rules are actually in the kernel. The check
	// at the top of this function was made before two file reads and a netlink
	// write; `panic` can have landed in between, from the console, in the very
	// window where this daemon has no socket for it to reach — see
	// panicLandedDuringWrite.
	if f.panicLandedDuringWrite(
		"boot_enforce_failed",
		fmt.Sprintf("%s: panic mode was engaged while the rules were being restored, "+
			"so the table was taken down again", reason),
		"core",
	) {
		// Not an error: the machine is in the state the console asked for. The
		// caller logs a failure and tells the operator to apply, which is the
		// wrong advice for a deliberately unfiltered machine.
		return nil
	}

	WriteAuditLog(f.cfg.AuditLogPath(), "boot_enforced", "all", reason, "core")
	slog.Info("the stored rules are in force again", "reason", reason)
	return nil
}

// panicLandedDuringWrite reports whether panic mode was engaged while a write to
// the kernel was in flight, and tears the table down again if it was. Call it
// immediately after nft.Apply returns; action and detail are what to record.
//
// Every other panic check in this package runs *before* a write and none ran
// after one, under a comment in cmd/easywall-core/subcommands.go asserting that
// "every daemon-side writer of the table checks it first" made a direct teardown
// survivable. Checking first is necessary and it is not sufficient, for two
// separate reasons.
//
// In-process, the gap is not small. Between apply's marker check and its
// nft.Apply sit two reads of the rules file, two atomic rewrites and a full
// Snapshot() — netlink list calls plus a file write. On the Raspberry Pi class
// of machine this product is written for that is tens to hundreds of
// milliseconds, not microseconds, and a `panic` at the console lands inside it
// often enough to matter.
//
// Cross-process, no mutex can help. `easywall-core panic` falls back to tearing
// the table down itself whenever the socket refuses, and there are two windows
// where a live daemon writes the kernel with no socket present: startup, because
// the boot restore deliberately runs before net.Listen, and shutdown, because
// Stop closes and unlinks the listener before CancelAcceptance() and wg.Wait().
// If the daemon's Flush lands after the CLI's teardown, the machine is filtering
// with the marker on disk — and every surface says the opposite, because
// runStatus tests status.Panic before status.Active and the web banner reads the
// same flag. That is the migration scenario exactly: somebody upgrading from
// 2.6, who reboots to escape their own rules, finds the rules restored, and
// reaches for `panic` while the machine is coming up.
//
// It also removes an audit-log lie. A panic landing mid-restore used to log
// panic_engaged and then boot_enforced — "the stored rules are in force again"
// as the last word on an unfiltered machine.
func (f *Firewall) panicLandedDuringWrite(action, detail, user string) bool {
	if !f.PanicEngaged() {
		return false
	}
	// PanicEngaged, not PanicState: this is the direction its fail-safe default
	// is built for. An unreadable marker here costs a teardown of a table that
	// is about to be rebuilt by the next apply or restore; the other way round
	// costs a machine that filters while the console believes it does not.
	if err := f.nft.Reset(); err != nil {
		slog.Error("panic mode was engaged while the rules were being written and the "+
			"table could not be torn down again; this machine may be filtering while "+
			"panic mode is recorded — run `nft delete table inet easywall`",
			"error", err)
		detail += "; the table could not be torn down: " + err.Error()
	}
	WriteAuditLog(f.cfg.AuditLogPath(), action, "all", detail, user)
	slog.Warn("panic mode was engaged while the rules were being written; the table has "+
		"been taken down again", "detail", detail)
	return true
}

// Panic tears the firewall down and records that it was done on purpose.
//
// The order is the whole design. The marker is written first and the table torn
// down second, because the failure that matters is the one where the operator
// runs this, believes it worked, reboots, and meets the rules that made them run
// it. A marker without a teardown leaves a machine that is still filtered and
// says so; a teardown without a marker leaves a machine that filters again at
// the next restart, silently.
//
// The same order also settles a race this function used to lose. An apply can
// be sitting in its acceptance window when this runs, blocked in Wait() on
// another goroutine. Cancelling that window does not stop its rollback —
// cancelling is what *starts* it: Wait() sees the cancel and returns false,
// and the caller runs rollback() as if the window had simply timed out. What
// stops that rollback from flushing the previous rules back into the kernel
// on top of the teardown below is Firewall.rollback refusing to act once the
// marker is set. For that guard to be reliable, the marker has to already be
// on disk by the time the cancel below can wake anything up — which is why
// EngagePanic and the audit entry that follows come first, and
// CancelAcceptance comes after them, never before.
func (f *Firewall) Panic(user string) error {
	if err := EngagePanic(f.cfg.PanicMarkerPath()); err != nil {
		return fmt.Errorf("engage panic mode: %w", err)
	}
	WriteAuditLog(f.cfg.AuditLogPath(), "panic_engaged", "all",
		"the firewall was taken down from the console", user)

	// Now that the marker rollback checks for is in place, ending an open
	// window is safe: whatever goroutine Wait() wakes up will find
	// PanicEngaged() true and refuse to touch the rules. Not cancelling here
	// would instead leave that window running for up to an hour, after which
	// the very rollback this ordering protects against would still run — just
	// late.
	f.CancelAcceptance()

	if err := f.nft.Reset(); err != nil {
		slog.Error("panic mode is recorded but the table could not be torn down; "+
			"the machine may still be filtering", "error", err)
		return fmt.Errorf("tear down the table: %w", err)
	}

	slog.Warn("panic mode: this machine is now unfiltered", "user", user)
	return nil
}

// Resume ends panic mode and puts the stored rules back.
//
// The marker is cleared before the restore is attempted, so a restore that fails
// does not leave a machine that is unfiltered *and* claims to be in panic mode —
// two different problems reported as one.
func (f *Firewall) Resume(user string) error {
	if err := ClearPanic(f.cfg.PanicMarkerPath()); err != nil {
		return fmt.Errorf("end panic mode: %w", err)
	}
	WriteAuditLog(f.cfg.AuditLogPath(), "panic_resumed", "all",
		"panic mode was ended from the console", user)

	err := f.RestoreCurrent(RestoreReasonResume)
	if errors.Is(err, ErrApplyInProgress) {
		// RestoreCurrent takes the same slot an apply does and refuses rather
		// than wait for it, which is correct — but until now that refusal
		// wrote nothing here. The operator was left with a log that said
		// panic mode had ended and no line anywhere saying the rules it was
		// supposed to bring back never arrived: the marker was gone, an apply
		// still held the slot, and the machine stayed unfiltered with nothing
		// on record to explain why.
		WriteAuditLog(f.cfg.AuditLogPath(), "resume_restore_skipped", "all",
			"an apply held the slot; the stored rules were not restored — resume again once it finishes",
			user)
	}
	return err
}
