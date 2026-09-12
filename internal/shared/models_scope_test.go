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

// omitempty, for the same reason ID has it: a rules.json written before 2.19
// has no scope key, and must come back out exactly as it went in until
// something touches it. Asserted as a whole round trip rather than as one
// field, because byte-identity of the file is the actual promise.
//
// An explicitly written "scope":"host" is kept as written. Normalising it away
// would need a custom marshaller on the struct every rule flows through, to buy
// a property nothing requires — and an operator's file is not ours to tidy.
func TestPortRule_AnUntouchedRuleRoundTripsUnchanged(t *testing.T) {
	const in = `{"port":"22","description":"SSH","ssh":true}`

	var r PortRule
	if err := json.Unmarshal([]byte(in), &r); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Errorf("a rule written before 2.19 did not survive a round trip:\n in: %s\nout: %s", in, out)
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
