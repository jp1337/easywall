package web

import (
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// notifyState remembers enough of the last status to tell a new outcome from
// the same one seen again.
//
// The identity of one apply cycle is (LastApply, Acceptance). Both accepted and
// rolled_back are terminal states that persist in the status rather than
// instants that pass, so a poller that wakes after the window closed still
// finds the outcome — there is no race against the 120-second window.
type notifyState struct {
	lastApply  string
	acceptance shared.AcceptanceStatus
	panicOn    bool
	// seeded is false until the first successful observation. The first status
	// a freshly started process sees describes what happened before it existed,
	// and announcing it would page an operator on every restart.
	seeded bool
}

func (s *notifyState) observe(status *shared.FirewallStatus) []Notification {
	// statusForRender returns nil when the core cannot be reached, and nil is
	// "unknown", not a state. Forgetting what we knew here would make a core
	// restart report a rollback that never happened.
	if status == nil {
		return nil
	}

	if !s.seeded {
		s.lastApply, s.acceptance, s.panicOn, s.seeded = status.LastApply, status.Acceptance, status.Panic, true
		return nil
	}

	var out []Notification
	now := time.Now().UTC()

	if status.LastApply != s.lastApply || status.Acceptance != s.acceptance {
		switch status.Acceptance {
		case shared.AcceptanceRolledBack:
			out = append(out, Notification{
				Event: "rolled_back", Severity: "warning",
				Detail: status.AcceptanceReason, Time: now,
			})
		case shared.AcceptanceAccepted:
			out = append(out, Notification{
				Event: "accepted", Severity: "info", Time: now,
			})
		}
	}

	if status.Panic != s.panicOn {
		// Critical on the way in, warning on the way out. notify.go's
		// ntfyPriority maps critical to ntfy's "5", and both that function's
		// comment and docs/_docs/features/notifications.md say the highest
		// priority is kept for the one event that means this host is not
		// filtering. "panic mode ended and the stored rules are back" is the
		// opposite of that, and it was paging operators at max priority with
		// good news. Still worth telling — the firewall's posture changed
		// without anyone touching the interface — so it is a warning, not info.
		detail, sev := "panic mode was engaged — this host is not filtering", "critical"
		if !status.Panic {
			detail, sev = "panic mode ended and the stored rules are back", "warning"
		}
		out = append(out, Notification{
			Event: "panic", Severity: sev, Detail: detail, Time: now,
		})
	}

	s.lastApply, s.acceptance, s.panicOn = status.LastApply, status.Acceptance, status.Panic
	return out
}
