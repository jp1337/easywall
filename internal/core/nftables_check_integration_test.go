//go:build integration

package core

import (
	"reflect"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// firewallOptionBools is how many boolean fields shared.FirewallOptions has:
// twelve protection modules and ten per-module log toggles.
//
// It is counted from the struct, not from the documentation. DESIGN.md calls the
// protection modules eleven in two places and fourteen in a third; resolving
// that contradiction is carried work and deliberately not settled here, so this
// gate asserts against the code and names the number it found. Whichever count
// DESIGN.md settles on, the guard below is about coverage: every boolean the
// struct has is switched on so every builder's output is read.
const firewallOptionBools = 22

// allProtectionModulesOn switches on every boolean option there is.
//
// Reflectively rather than as a literal, so a module added in a later release
// is covered by this gate the day it lands. A literal would need a line adding
// beside it, and a builder whose output nothing reads is exactly how three
// byte-reversed conntrack masks survived five releases — the omission is never
// in the line somebody wrote, it is in the line nobody thought to.
//
// The numeric limits are left at zero on purpose: every builder substitutes its
// own default for a non-positive limit, so zero exercises that path too.
func allProtectionModulesOn() shared.FirewallOptions {
	var opts shared.FirewallOptions
	v := reflect.ValueOf(&opts).Elem()
	for i := 0; i < v.NumField(); i++ {
		if f := v.Field(i); f.Kind() == reflect.Bool && f.CanSet() {
			f.SetBool(true)
		}
	}
	return opts
}

// countTrueBoolFields counts the boolean fields of opts that are set.
func countTrueBoolFields(opts shared.FirewallOptions) int {
	v := reflect.ValueOf(opts)
	n := 0
	for i := 0; i < v.NumField(); i++ {
		if f := v.Field(i); f.Kind() == reflect.Bool && f.Bool() {
			n++
		}
	}
	return n
}

// fullExampleRules is a rule set that reaches every builder Apply can call.
//
// One entry per shape rather than a realistic policy: an SSH-marked port so
// addSSHBruteForce meters something, a range so buildPortExprs takes its second
// path, a source-restricted rule, a UDP port, a blacklist and a whitelist entry,
// a private whitelist network so the bogon filter builds its exemption returns,
// and a forward so the NAT prerouting chain exists. Custom rules are left out:
// they go through the nft CLI after the flush and never become expressions this
// check can read.
func fullExampleRules() shared.Rules {
	return shared.Rules{
		TCP: []shared.PortRule{
			{Port: "22", Description: "ssh", SSH: true},
			{Port: "8000:9000", Description: "a range"},
			{Port: "443", Description: "restricted", Sources: []string{"203.0.113.0/24"}},
		},
		UDP:        []shared.PortRule{{Port: "53", Description: "dns"}},
		Blacklist:  []string{"198.51.100.7"},
		Whitelist:  []string{"192.168.42.0/24"},
		Forwarding: []shared.ForwardingRule{{Protocol: "tcp", SourcePort: 2222, DestPort: 22}},
		Custom:     []string{},
	}
}

// The gate. A finding in the table easywall actually builds is a build failure,
// not a warning: every host would get it, and the last one of this class
// shipped for five releases while every surface reported the firewall active.
//
// On a host a finding logs and degrades health and the table is written anyway
// — see checkBuilt. Here is where it is fatal, which is the half that kills the
// class rather than reporting it.
func TestIntegration_TheBuiltTableHasNoFindings(t *testing.T) {
	m := newIntegrationManager(t)

	opts := allProtectionModulesOn()
	// A count rather than a spot check, and fatal rather than skipped: a gate
	// that runs with the modules off proves nothing, and a skip is
	// indistinguishable from a pass in CI output.
	if got := countTrueBoolFields(opts); got != firewallOptionBools {
		t.Fatalf("allProtectionModulesOn set %d of shared.FirewallOptions' %d "+
			"boolean fields; a field was added or the reflection stopped "+
			"reaching them, and the builders behind it would go unchecked",
			got, firewallOptionBools)
	}

	state := shared.RulesState{Current: fullExampleRules(), Staged: fullExampleRules()}
	if err := m.Apply(state, opts, shared.NetworkSettings{
		Routing: shared.RoutingConfig{Mode: shared.RoutingNetworks, Networks: []string{"10.9.0.0/24"}},
		IPv6:    shared.IPv6Config{Mode: shared.IPv6Filter},
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Every module's rules have to have reached m.built, or an empty check
	// would pass for the wrong reason. The exact number is not the point and
	// would need updating for every new rule; that it is in the hundreds is.
	if n := len(m.built); n < 50 {
		t.Fatalf("only %d rules were recorded for the check; the recording "+
			"adder is not in the path and the gate is proving nothing", n)
	}

	for _, f := range m.LastFindings() {
		t.Errorf("finding: %s", f)
	}
}
