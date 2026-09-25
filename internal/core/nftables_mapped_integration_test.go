//go:build integration

package core

import (
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// An IPv4-mapped network is an IPv4 network written in the other family's
// notation, and validation has always accepted it. Until 2.22 no builder
// turned it back into IPv4: in the blocklist and the allowlist it produced a
// 4-byte address compare behind a 16-byte mask, and the kernel refused the
// whole rule set with EINVAL; in a port rule's sources it was skipped with a
// WARN, so the rule opened nothing. Each case here names where the entry sits
// and the line `nft list` must print for it.
func TestIntegration_AMappedNetworkIsWrittenAsTheIPv4NetworkItNames(t *testing.T) {
	const mapped = "::ffff:10.0.0.0/104"
	cases := []struct {
		name  string
		state func(*shared.RulesState)
		opts  shared.FirewallOptions
		chain string
		want  []string
	}{
		{"blocklist", func(s *shared.RulesState) { s.Current.Blocklist = []string{mapped} },
			shared.FirewallOptions{}, "input", []string{"ip saddr 10.0.0.0/8", "drop"}},
		{"allowlist", func(s *shared.RulesState) { s.Current.Allowlist = []string{mapped} },
			shared.FirewallOptions{}, "input", []string{"ip saddr 10.0.0.0/8", "accept"}},
		{"allowlist, as a bogon exception", func(s *shared.RulesState) { s.Current.Allowlist = []string{mapped} },
			shared.FirewallOptions{Bogons: true}, "bogon", []string{"ip saddr 10.0.0.0/8", "return"}},
		{"port source", func(s *shared.RulesState) {
			s.Current.TCP = []shared.PortRule{{Port: "2222", Sources: []string{mapped}}}
		}, shared.FirewallOptions{}, "input", []string{"ip saddr 10.0.0.0/8", "tcp dport 2222", "accept"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newIntegrationManager(t)
			state := emptyState()
			tc.state(&state)
			if err := m.Apply(state, tc.opts, shared.NetworkSettings{}); err != nil {
				t.Fatalf("Apply with %s in the %s: %v", mapped, tc.name, err)
			}
			rules := chainText(t, tc.chain)
			if indexOfRule(rules, tc.want...) < 0 {
				t.Errorf("chain %s has no rule with %q:\n%v", tc.chain, tc.want, rules)
			}
		})
	}
}

// The published-port warning filters Docker's DNAT targets by the same network
// list, so a mapped custom network has to name the containers its IPv4 form
// names. Until 2.22 it named none, and the warning stayed silent about them.
func TestIntegration_AMappedNetworkFindsThePublishedPortsItsIPv4NetworkFinds(t *testing.T) {
	dockerNATFixture(t)

	want := detectPublishedPorts([]string{"172.18.0.0/16"}, 0)
	if len(want) == 0 {
		skipOrFailUnprovable(t, "the fixture is not readable, so an empty answer would prove nothing")
	}
	got := detectPublishedPorts([]string{"::ffff:172.18.0.0/112"}, 0)
	if len(got) != len(want) {
		t.Errorf("::ffff:172.18.0.0/112 found %d published ports, 172.18.0.0/16 finds %d: %v",
			len(got), len(want), got)
	}
}
