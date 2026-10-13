package core

import (
	"log/slog"
	"sync"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// Acceptance manages the two-step activation safety mechanism.
// When rules are applied, the admin has a configurable window to confirm
// the new rules work (e.g. SSH still connects). If confirmation is not
// received within the timeout, rules are automatically rolled back.
type Acceptance struct {
	mu        sync.Mutex
	status    shared.AcceptanceStatus
	acceptCh  chan struct{}
	cancelCh  chan struct{}
	cancelled bool
	// accepted is the decision, made under mu by Accept. Wait reads it after
	// waking rather than trusting which select case won: with a cancel, a timer
	// and an accept all ready, select picks at random, and Accept used to report
	// success for a window Wait then rolled back.
	accepted bool
	duration time.Duration

	// deadline is when the open window ends. Set by Start, read by Wait and by
	// Remaining, so the timer that fires and the number on the screen are one
	// value rather than two derivations of a duration that might disagree.
	deadline time.Time

	// stopping records that the daemon is shutting down, so a window that has
	// not opened yet opens already cancelled. Sticky on purpose and set only by
	// CancelForShutdown: a process that is stopping is not going to start
	// applying again, and Panic — which also cancels — must not set it.
	stopping bool

	// reason records why the window ended, for the audit line Firewall.apply
	// writes. It defaults to a timeout because that is what a window that nobody
	// touches does; a cancel overrides it with who ended it — the operator,
	// panic mode, or the core stopping.
	//
	// It exists because the detail on that line used to be the literal string
	// "timeout", written by apply on every path out of Wait. A rollback the
	// operator asked for was therefore recorded as a window that expired, which is
	// the same shape of lie as reporting a confirmation that arrived too late as a
	// success.
	reason string
}

// NewAcceptance creates a new Acceptance controller with the given timeout.
func NewAcceptance(duration time.Duration) *Acceptance {
	return &Acceptance{
		status:   shared.AcceptanceIdle,
		duration: duration,
	}
}

// Status returns the current acceptance status (thread-safe).
func (a *Acceptance) Status() shared.AcceptanceStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

// Start begins a new acceptance window of the given length. It must be called
// before nftables rules are applied.
//
// It returns nil, always, and starting while a window is already open is a
// no-op rather than an error — TestAcceptance_StartIdempotent pins that. The
// comment here used to promise the opposite ("returns an error if an acceptance
// is already in progress"), which made the error check at the one call site in
// Firewall.apply read as the guard against a second apply. It is not: that guard
// is beginApply, which claims the slot synchronously and refuses with
// ErrApplyInProgress. The error return is kept because a future window that
// cannot be opened is a real possibility, but nothing produces one today.
//
// The length is passed in per apply rather than fixed when the controller is
// built. It used to be captured once, at daemon start, so changing the duration
// on the system settings page wrote the new value to easywall.toml and every
// apply for the rest of the process's life still used the old one — while the
// documentation said the next apply would pick it up.
func (a *Acceptance) Start(duration time.Duration) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.status == shared.AcceptancePending {
		return nil // idempotent
	}

	if duration > 0 {
		a.duration = duration
	}
	a.acceptCh = make(chan struct{}, 1)
	a.cancelCh = make(chan struct{})
	a.cancelled = false
	a.accepted = false
	a.status = shared.AcceptancePending
	a.deadline = time.Now().Add(a.duration)

	// Every window starts as a timeout until something says otherwise. Set here,
	// not once at construction, so a second window does not inherit whatever the
	// first one was cancelled with.
	a.reason = ReasonTimeout

	// The shutdown that already happened. Opening the window pre-cancelled is
	// what makes "the machine was told to stop" mean "nobody confirmed", which
	// is what the window promises: Wait returns false at once and the caller
	// rolls back, rather than holding Stop for up to an hour.
	if a.stopping {
		a.cancelled = true
		a.reason = ReasonCoreStopped
		close(a.cancelCh)
	}
	return nil
}

// Duration returns the length the window was asked to be.
//
// One caller, deliberately: the "duration" field on the timeout log line, where
// how long the window was is worth its two lines. Nothing may derive a *time*
// from it — Wait times from the deadline, and Remaining reports from the same
// instant, so the count on screen and the rollback that fires are one value.
func (a *Acceptance) Duration() time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.duration
}

// Remaining is what is left of the open window: zero when none is open, and
// never negative.
//
// The web process renders this as mm:ss on every page. Both clamps are load-
// bearing — an idle controller holds the previous window's deadline, and a
// deadline a fraction of a second in the past renders as a negative clock on
// the one screen whose argument is that it says what is true.
func (a *Acceptance) Remaining() time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.status != shared.AcceptancePending {
		return 0
	}
	if d := time.Until(a.deadline); d > 0 {
		return d
	}
	return 0
}

// Wait blocks until the admin calls Accept() or the timeout expires.
// Returns true if accepted, false if timed out (rollback needed).
func (a *Acceptance) Wait() bool {
	a.mu.Lock()
	acceptCh, cancelCh, deadline := a.acceptCh, a.cancelCh, a.deadline
	a.mu.Unlock()

	// From the deadline, never from Duration(). Duration is what the window was
	// asked to be; the deadline is what it is, and it is also the number the
	// interface renders — one instant, so the count on the screen and the
	// rollback that fires cannot be two different clocks. A zero deadline means
	// Wait was reached without a Start: time.Until is then hugely negative, the
	// timer fires at once and the rules roll back, which is the safe direction.
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()

	var cancelled, timedOut bool
	select {
	case <-cancelCh:
		cancelled = true
	case <-acceptCh:
	case <-timer.C:
		timedOut = true
	}

	// The outcome is what Accept decided under the lock, not which case won.
	// Accept refuses once the window is cancelled or past its deadline, and
	// cancel refuses once it is accepted, so the flag is the one answer both
	// sides agree on.
	a.mu.Lock()
	accepted := a.accepted
	if accepted {
		a.status = shared.AcceptanceAccepted
	} else {
		a.status = shared.AcceptanceRolledBack
	}
	a.mu.Unlock()

	switch {
	case accepted:
		slog.Info("rules accepted by user")
	case cancelled:
		slog.Warn("acceptance cancelled — rolling back rules", "reason", a.Reason())
	case timedOut:
		slog.Warn("acceptance timed out — rolling back rules",
			"duration", a.Duration())
	}
	return accepted
}

// Cancel ends a pending window as *not* accepted, so Wait returns false and the
// caller rolls back. It reports whether there was an open window to cancel.
//
// Production code uses the named variants — CancelByOperator, CancelForPanic,
// CancelForShutdown — so the audit line says who ended the window; this one
// records no reason and is left for tests that only need "nobody confirmed".
//
// Cancelling exists for shutdown. An apply's window can stay open for up to an hour,
// and until the daemon waited for it, stopping in the middle — a package
// upgrade, a systemctl restart, a SIGTERM — simply abandoned the goroutine
// holding it. The unconfirmed rules stayed live and the rollback that is the
// whole promise of the window never ran. Not confirming has to mean the old
// rules come back, including when the reason nobody confirmed is that the
// machine was told to stop.
//
// The return value matters just as much as Accept()'s does, and for the same
// reason: a rollback that arrives after the window has already closed changed
// nothing — the previous rules are already back, on their own — and reporting
// it as a success would tell the caller they acted at the one moment they did
// not.
func (a *Acceptance) Cancel() bool { return a.cancel("") }

// The reasons a window ends with, as the audit detail and the status report
// them. Tokens, not sentences: the web process translates each one through
// auditDetailKeys, and apply.html branches on them.
const (
	ReasonTimeout     = "timeout"
	ReasonOperator    = "cancelled by operator"
	ReasonPanic       = "panic mode"
	ReasonCoreStopped = "core stopped"
)

// CancelByOperator is Cancel with the reason the audit line will carry: it
// cancels the open window on the operator's own request and, only if there
// truly was one, records that as why it ended.
func (a *Acceptance) CancelByOperator() bool { return a.cancel(ReasonOperator) }

// cancel ends the window and, when the caller named one, records why — both
// under the same lock. The reason may not be written before the window is
// known to be open: Wait's timeout branch sets the status and then logs
// before returning, and a cancel arriving in that gap would stamp "cancelled
// by operator" onto a window that had just genuinely expired — the one
// mislabel this command exists to prevent. A prior version set the reason in
// CancelByOperator before taking this lock, on the claim that a stale reason
// is never read because the next Start resets it; that claim was false for
// exactly this gap, where apply reads Reason() before any Start runs again.
func (a *Acceptance) cancel(reason string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Closed once, and never set to nil: Wait may not have captured it yet, and
	// a nil channel blocks forever, which would turn a cancel into a full-length
	// window — the opposite of what shutdown needs.
	//
	// Not after an accept, and not past the deadline: either way the window has
	// already ended, and reporting a cancel would claim a rollback that is not
	// the one happening.
	if a.status != shared.AcceptancePending || a.cancelled || a.accepted ||
		a.cancelCh == nil || !time.Now().Before(a.deadline) {
		return false
	}
	if reason != "" {
		a.reason = reason
	}
	a.cancelled = true
	close(a.cancelCh)
	return true
}

// Reason reports why the current or most recently ended window ended: one of
// the Reason* tokens above. It is what Firewall.apply writes as
// the audit detail for apply_rolledback, so that one action carries two
// distinguishable stories instead of the literal string "timeout" it used to
// write on every path out of Wait.
func (a *Acceptance) Reason() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.reason
}

// CancelForShutdown ends an open window and guarantees that no later one opens.
//
// Cancel alone is not enough on the shutdown path. Stop cancels and then waits
// on the WaitGroup that tracks the apply goroutine, and that goroutine only
// reaches Start after a rules read, a backup write, an nft snapshot subprocess
// and a promote — so the cancel routinely arrives while the status is still
// Idle, where Cancel is a no-op. The cancel was then lost, the window opened
// anyway, and Stop sat behind Wait for the full duration until systemd's
// TimeoutStopSec SIGKILLed the daemon with the unconfirmed rules live.
func (a *Acceptance) CancelForShutdown() {
	a.mu.Lock()
	a.stopping = true
	a.mu.Unlock()
	a.cancel(ReasonCoreStopped)
}

// CancelForPanic ends an open window because panic mode was engaged, and
// records that as why. It used to be plain Cancel, which recorded nothing, so
// a window ended by `easywall-core panic` — or by a shutdown — reached the
// audit log as "timeout": the very mislabel the reason field exists to remove.
func (a *Acceptance) CancelForPanic() bool { return a.cancel(ReasonPanic) }

// RecordRolledBack marks the controller as having ended a window unconfirmed,
// with no window open in this process. The boot restore uses it when it finds
// a window a previous run of the core left open, so the status reports the
// rollback it just carried out rather than idle.
func (a *Acceptance) RecordRolledBack(reason string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status = shared.AcceptanceRolledBack
	a.reason = reason
}

// Accept signals that the admin confirmed the new rules work, and reports
// whether there was an open window to accept.
//
// The answer matters. A confirmation that arrives a second after the window
// closed was discarded silently, and the interface said "Rules accepted and
// applied successfully" anyway — telling the operator their change is live at
// the one moment it is not, because it has just been rolled back.
//
// Non-blocking: safe to call from a different goroutine.
func (a *Acceptance) Accept() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	//
	// Pending is not enough. A window that was cancelled, or whose deadline has
	// passed, stays Pending until Wait wakes and takes this lock — and in that
	// gap Accept used to answer true for a window that was rolling back (516 of
	// 1000 in a probe that accepted right after a cancel).
	if a.status != shared.AcceptancePending || a.cancelled || !time.Now().Before(a.deadline) {
		return false
	}
	a.accepted = true
	select {
	case a.acceptCh <- struct{}{}:
	default:
		// Already signalled; the window is being accepted either way.
	}
	return true
}

// Reset returns the acceptance state to idle. Used for a window that never got
// as far as an outcome — an apply that failed or was overtaken by panic mode
// before Wait. A window Wait closed keeps its outcome until the next Start:
// see Firewall.apply for what clearing it cost.
func (a *Acceptance) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status = shared.AcceptanceIdle
}
