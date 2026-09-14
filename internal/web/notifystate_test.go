package web

import (
	"reflect"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestObserveTurnsStatusIntoNotifications(t *testing.T) {
	st := func(apply string, a shared.AcceptanceStatus, p bool, reason string) *shared.FirewallStatus {
		return &shared.FirewallStatus{LastApply: apply, Acceptance: a, Panic: p, AcceptanceReason: reason}
	}
	cases := []struct {
		name    string
		steps   []*shared.FirewallStatus
		events  []string // events from the LAST step only
		details []string // each event's Detail, same order as events
		sevs    []string // each event's Severity, same order as events
	}{
		{"the first observation seeds and announces nothing",
			[]*shared.FirewallStatus{st("t1", shared.AcceptanceRolledBack, false, "timeout")}, nil, nil, nil},
		{"a new rollback after seeding",
			[]*shared.FirewallStatus{st("t1", shared.AcceptanceIdle, false, "timeout"), st("t2", shared.AcceptanceRolledBack, false, "timeout")},
			[]string{"rolled_back"}, []string{"timeout"}, []string{"warning"}},
		{"the same rollback seen twice says it once",
			[]*shared.FirewallStatus{st("t1", shared.AcceptanceIdle, false, "timeout"), st("t2", shared.AcceptanceRolledBack, false, "timeout"), st("t2", shared.AcceptanceRolledBack, false, "timeout")},
			nil, nil, nil},
		// AcceptanceReason distinguishes "the window expired" from "an operator
		// ended it by hand" — exercise the half "timeout" everywhere else in
		// this table never reaches, and prove it lands in Detail unflattened.
		{"a rollback ended by the operator carries that reason, not the timeout wording",
			[]*shared.FirewallStatus{st("t1", shared.AcceptanceIdle, false, "timeout"), st("t2", shared.AcceptanceRolledBack, false, "operator ended it by hand")},
			[]string{"rolled_back"}, []string{"operator ended it by hand"}, []string{"warning"}},
		{"a confirmed apply",
			[]*shared.FirewallStatus{st("t1", shared.AcceptanceIdle, false, "timeout"), st("t2", shared.AcceptanceAccepted, false, "timeout")},
			[]string{"accepted"}, []string{""}, []string{"info"}},
		{"panic engaged",
			[]*shared.FirewallStatus{st("t1", shared.AcceptanceIdle, false, "timeout"), st("t1", shared.AcceptanceIdle, true, "timeout")},
			[]string{"panic"}, []string{"panic mode was engaged — this host is not filtering"}, []string{"critical"}},
		{"panic ended",
			[]*shared.FirewallStatus{st("t1", shared.AcceptanceIdle, true, "timeout"), st("t1", shared.AcceptanceIdle, false, "timeout")},
			[]string{"panic"}, []string{"panic mode ended and the stored rules are back"}, []string{"critical"}},
		// The row the spec insists on: an unreachable core must not manufacture
		// an event, and must not forget what it knew.
		{"a nil status changes nothing and is not an event",
			[]*shared.FirewallStatus{st("t1", shared.AcceptanceIdle, false, "timeout"), nil}, nil, nil, nil},
		// This is the row that actually proves survival: it re-observes the same
		// ("t2", rolled_back) already seeded two steps earlier, with a nil in
		// between. If observe(nil) had clobbered the remembered state (e.g. to
		// the zero value ""), this repeat would look "new" again — Acceptance
		// "" != AcceptanceRolledBack — and would wrongly re-announce it. A
		// follow-up of AcceptanceIdle cannot catch that: "idle" matches neither
		// switch case, so a corrupted-to-zero-value state produces the same
		// (nil) result as a correct one either way.
		{"the state survives the nil: a repeated rollback across an intervening nil is not re-announced",
			[]*shared.FirewallStatus{
				st("t1", shared.AcceptanceIdle, false, "timeout"),
				st("t2", shared.AcceptanceRolledBack, false, "timeout"),
				nil,
				st("t2", shared.AcceptanceRolledBack, false, "timeout"),
			}, nil, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var s notifyState
			var last []Notification
			for _, step := range tc.steps {
				last = s.observe(step)
			}
			gotEvents := make([]string, 0, len(last))
			gotDetails := make([]string, 0, len(last))
			gotSevs := make([]string, 0, len(last))
			for _, n := range last {
				gotEvents = append(gotEvents, n.Event)
				gotDetails = append(gotDetails, n.Detail)
				gotSevs = append(gotSevs, n.Severity)
			}
			if !equalOrBothEmpty(gotEvents, tc.events) {
				t.Errorf("events = %v, want %v", gotEvents, tc.events)
			}
			if !equalOrBothEmpty(gotDetails, tc.details) {
				t.Errorf("details = %v, want %v", gotDetails, tc.details)
			}
			if !equalOrBothEmpty(gotSevs, tc.sevs) {
				t.Errorf("severities = %v, want %v", gotSevs, tc.sevs)
			}
		})
	}
}

// equalOrBothEmpty treats nil and an empty slice as equal — reflect.DeepEqual
// does not, and the table above uses nil for "no events" throughout.
func equalOrBothEmpty(got, want []string) bool {
	if len(got) == 0 && len(want) == 0 {
		return true
	}
	return reflect.DeepEqual(got, want)
}
