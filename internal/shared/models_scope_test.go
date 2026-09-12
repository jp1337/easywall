package shared

import (
	"encoding/json"
	"testing"
)

// An empty scope is a host rule. This is the promise that a rules.json written
// before 2.19 means after it exactly what it meant before, and it is asserted
// through the two predicates rather than through the field, because the
// predicates are what the rule builder asks.
func TestPortRule_AnAbsentScopeIsAHostRule(t *testing.T) {
	var r PortRule
	if err := json.Unmarshal([]byte(`{"port":"22","description":"SSH","ssh":true}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.Scope != "" {
		t.Errorf("a rule with no scope key parsed as %q, not empty", r.Scope)
	}
	if !r.FiltersHost() {
		t.Error("a rule with no scope must filter the host chain")
	}
	if r.FiltersForwarded() {
		t.Error("a rule with no scope must not filter the forward chain")
	}
}

// omitempty, for the same reason ID has it: a file written before this release
// stays byte-identical until something touches it.
func TestPortRule_AHostScopeIsNotWrittenBack(t *testing.T) {
	out, err := json.Marshal(PortRule{Port: "22", Description: "SSH", Scope: ScopeHost})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"port":"22","description":"SSH","ssh":false}` {
		t.Errorf("an explicit host scope was written back: %s", out)
	}
}

func TestPortRule_ScopePredicates(t *testing.T) {
	for _, tc := range []struct {
		scope           PortScope
		host, forwarded bool
	}{
		{"", true, false},
		{ScopeHost, true, false},
		{ScopeForwarded, false, true},
		{ScopeBoth, true, true},
	} {
		r := PortRule{Port: "25", Scope: tc.scope}
		if got := r.FiltersHost(); got != tc.host {
			t.Errorf("scope %q: FiltersHost()=%v, want %v", tc.scope, got, tc.host)
		}
		if got := r.FiltersForwarded(); got != tc.forwarded {
			t.Errorf("scope %q: FiltersForwarded()=%v, want %v", tc.scope, got, tc.forwarded)
		}
	}
}
