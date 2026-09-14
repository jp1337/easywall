package web

import (
	"reflect"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestObserveTurnsStatusIntoNotifications(t *testing.T) {
	st := func(apply string, a shared.AcceptanceStatus, p bool) *shared.FirewallStatus {
		return &shared.FirewallStatus{LastApply: apply, Acceptance: a, Panic: p, AcceptanceReason: "timeout"}
	}
	cases := []struct {
		name   string
		steps  []*shared.FirewallStatus
		events []string // events from the LAST step only
	}{
		{"the first observation seeds and announces nothing",
			[]*shared.FirewallStatus{st("t1", shared.AcceptanceRolledBack, false)}, nil},
		{"a new rollback after seeding",
			[]*shared.FirewallStatus{st("t1", shared.AcceptanceIdle, false), st("t2", shared.AcceptanceRolledBack, false)},
			[]string{"rolled_back"}},
		{"the same rollback seen twice says it once",
			[]*shared.FirewallStatus{st("t1", shared.AcceptanceIdle, false), st("t2", shared.AcceptanceRolledBack, false), st("t2", shared.AcceptanceRolledBack, false)},
			nil},
		{"a confirmed apply",
			[]*shared.FirewallStatus{st("t1", shared.AcceptanceIdle, false), st("t2", shared.AcceptanceAccepted, false)},
			[]string{"accepted"}},
		{"panic engaged",
			[]*shared.FirewallStatus{st("t1", shared.AcceptanceIdle, false), st("t1", shared.AcceptanceIdle, true)},
			[]string{"panic"}},
		{"panic ended",
			[]*shared.FirewallStatus{st("t1", shared.AcceptanceIdle, true), st("t1", shared.AcceptanceIdle, false)},
			[]string{"panic"}},
		// The row the spec insists on: an unreachable core must not manufacture
		// an event, and must not forget what it knew.
		{"a nil status changes nothing and is not an event",
			[]*shared.FirewallStatus{st("t1", shared.AcceptanceIdle, false), nil}, nil},
		{"and the state survives the nil",
			[]*shared.FirewallStatus{st("t1", shared.AcceptanceIdle, false), nil, st("t1", shared.AcceptanceIdle, false)}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var s notifyState
			var last []Notification
			for _, step := range tc.steps {
				last = s.observe(step)
			}
			got := make([]string, 0, len(last))
			for _, n := range last {
				got = append(got, n.Event)
			}
			if !reflect.DeepEqual(got, tc.events) && !(len(got) == 0 && len(tc.events) == 0) {
				t.Errorf("events = %v, want %v", got, tc.events)
			}
		})
	}
}
