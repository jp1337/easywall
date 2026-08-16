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
	return nil
}

// Duration returns the window length the next Wait will use.
func (a *Acceptance) Duration() time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.duration
}

// Wait blocks until the admin calls Accept() or the timeout expires.
// Returns true if accepted, false if timed out (rollback needed).
func (a *Acceptance) Wait() bool {
	timer := time.NewTimer(a.Duration())
	defer timer.Stop()

	a.mu.Lock()
	acceptCh, cancelCh := a.acceptCh, a.cancelCh
	a.mu.Unlock()

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
// caller rolls back.
//
// It exists for shutdown. An apply's window can stay open for up to an hour,
// and until the daemon waited for it, stopping in the middle — a package
// upgrade, a systemctl restart, a SIGTERM — simply abandoned the goroutine
// holding it. The unconfirmed rules stayed live and the rollback that is the
// whole promise of the window never ran. Not confirming has to mean the old
// rules come back, including when the reason nobody confirmed is that the
// machine was told to stop.
func (a *Acceptance) Cancel() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Closed once, and never set to nil: Wait may not have captured it yet, and
	// a nil channel blocks forever, which would turn a cancel into a full-length
	// window — the opposite of what shutdown needs.
	if a.status != shared.AcceptancePending || a.cancelled || a.cancelCh == nil {
		return
	}
	a.cancelled = true
	close(a.cancelCh)
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
