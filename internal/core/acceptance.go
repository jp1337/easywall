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
	mu       sync.Mutex
	status   shared.AcceptanceStatus
	acceptCh chan struct{}
	duration time.Duration
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
// before nftables rules are applied. Returns an error if an acceptance is
// already in progress.
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

	select {
	case <-a.acceptCh:
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

// Accept signals that the admin confirmed the new rules work.
// Non-blocking: safe to call from a different goroutine.
func (a *Acceptance) Accept() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.status != shared.AcceptancePending {
		return
	}
	select {
	case a.acceptCh <- struct{}{}:
	default:
	}
}

// Reset returns the acceptance state to idle after a completed cycle.
func (a *Acceptance) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status = shared.AcceptanceIdle
}
