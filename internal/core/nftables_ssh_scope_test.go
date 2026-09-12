package core

import (
	"slices"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The SSH limiter lives in the input chain, so which ports it meters is a
// question about which rules reach that chain. A forwarded-only rule does not,
// and the empty-list fallback turns that omission from inert into harmful: the
// rule's port takes the place of the default port-22 protection, and the
// operator loses brute-force protection on the port they actually log in on.
func TestSSHMeteredPorts_IgnoresRulesThatNeverReachTheInputChain(t *testing.T) {
	for _, tc := range []struct {
		name string
		rule shared.PortRule
		want []string
	}{
		{"forwarded only", shared.PortRule{Port: "2222", SSH: true, Scope: shared.ScopeForwarded},
			[]string{"22"}},
		{"host", shared.PortRule{Port: "2222", SSH: true, Scope: shared.ScopeHost},
			[]string{"2222"}},
		{"default scope", shared.PortRule{Port: "2222", SSH: true},
			[]string{"2222"}},
		{"both", shared.PortRule{Port: "2222", SSH: true, Scope: shared.ScopeBoth},
			[]string{"2222"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := sshMeteredPorts(shared.Rules{TCP: []shared.PortRule{tc.rule}})
			if !slices.Equal(got, tc.want) {
				t.Errorf("metered ports = %v, want %v — a rule that does not reach the "+
					"input chain must neither be metered there nor displace the default",
					got, tc.want)
			}
		})
	}
}
