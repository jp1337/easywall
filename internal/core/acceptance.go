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
	duration  time.Duration

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
	// touches does, and only an operator-requested rollback overrides it.
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
	a.status = shared.AcceptancePending
	a.deadline = time.Now().Add(a.duration)

	// Every window starts as a timeout until something says otherwise. Set here,
	// not once at construction, so a second window does not inherit whatever the
	// first one was cancelled with.
	a.reason = "timeout"

	// The shutdown that already happened. Opening the window pre-cancelled is
	// what makes "the machine was told to stop" mean "nobody confirmed", which
	// is what the window promises: Wait returns false at once and the caller
	// rolls back, rather than holding Stop for up to an hour.
	if a.stopping {
		a.cancelled = true
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

	select {
	case <-cancelCh:
		a.mu.Lock()
		a.status = shared.AcceptanceRolledBack
		a.mu.Unlock()
		slog.Warn("acceptance cancelled — rolling back rules")
		return false
	case <-acceptCh:
		a.mu.Lock()
		a.status = shared.AcceptanceAccepted
		a.mu.Unlock()
		slog.Info("rules accepted by user")
		return true
	case <-timer.C:
		a.mu.Lock()
		a.status = shared.AcceptanceRolledBack
		a.mu.Unlock()
		slog.Warn("acceptance timed out — rolling back rules",
			"duration", a.Duration())
		return false
	}
}

// Cancel ends a pending window as *not* accepted, so Wait returns false and the
// caller rolls back. It reports whether there was an open window to cancel.
//
// It exists for shutdown. An apply's window can stay open for up to an hour,
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
func (a *Acceptance) Cancel() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Closed once, and never set to nil: Wait may not have captured it yet, and
	// a nil channel blocks forever, which would turn a cancel into a full-length
	// window — the opposite of what shutdown needs.
	if a.status != shared.AcceptancePending || a.cancelled || a.cancelCh == nil {
		return false
	}
	a.cancelled = true
	close(a.cancelCh)
	return true
}

// CancelByOperator cancels the open window on the operator's own request, and
// records that as the reason the window ended, for the audit line apply
// writes. It reports whether there was a window to cancel, exactly as Cancel
// does — CancelByOperator is Cancel, with the reason recorded first.
//
// The reason is set before the cancel, not after: if Cancel finds nothing to
// cancel, the reason is left stale, but nobody reads it until the next Start,
// which resets it — so no rollback of the write is needed.
func (a *Acceptance) CancelByOperator() bool {
	a.mu.Lock()
	a.reason = "cancelled by operator"
	a.mu.Unlock()
	return a.Cancel()
}

// Reason reports why the current or most recently ended window ended: either
// "timeout" or "cancelled by operator". It is what Firewall.apply writes as
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
	a.Cancel()
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

	if a.status != shared.AcceptancePending {
		return false
	}
	select {
	case a.acceptCh <- struct{}{}:
		return true
	default:
		// Already signalled; the window is being accepted either way.
		return true
	}
}

// Reset returns the acceptance state to idle after a completed cycle.
func (a *Acceptance) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status = shared.AcceptanceIdle
}
