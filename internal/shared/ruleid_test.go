package shared

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// Twelve hex characters, from crypto/rand. Not a UUID — thirty-six characters
// in every export buys nothing here — and not a counter, which would need a
// high-water mark stored beside the rules.
func TestNewRuleID_IsTwelveHexCharacters(t *testing.T) {
	shape := regexp.MustCompile(`^[0-9a-f]{12}$`)
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewRuleID()
		if !shape.MatchString(id) {
			t.Fatalf("NewRuleID() = %q, want twelve lowercase hex characters", id)
		}
		if seen[id] {
			t.Fatalf("NewRuleID() returned %q twice in 1000 draws; it is not random", id)
		}
		seen[id] = true
	}
}

// The field is what the usage counters are keyed by. A rule that already has
// one keeps it, whatever else about the rule changed — that is the whole
// promise, and the counter history rests on it.
func TestEnsureRuleIDs_FillsTheEmptyOnesAndTouchesNothingElse(t *testing.T) {
	r := Rules{
		TCP: []PortRule{
			{Port: "22", ID: "aaaaaaaaaaaa"},
			{Port: "80"},
		},
		UDP: []PortRule{{Port: "53"}},
	}
	if !EnsureRuleIDs(&r) {
		t.Fatal("EnsureRuleIDs reported no change, but two rules had no id")
	}
	if r.TCP[0].ID != "aaaaaaaaaaaa" {
		t.Errorf("the existing id was rewritten to %q; a counter keyed to the old one is orphaned", r.TCP[0].ID)
	}
	if r.TCP[1].ID == "" || r.UDP[0].ID == "" {
		t.Error("an empty id survived EnsureRuleIDs")
	}
	if r.TCP[1].ID == r.UDP[0].ID {
		t.Error("two rules were given the same id")
	}
	if EnsureRuleIDs(&r) {
		t.Error("a second call changed something; EnsureRuleIDs is not idempotent")
	}
}

// An import can carry two rules with the same id — a file edited by hand, or
// one rule copied over another. The first occurrence keeps it; the rest are
// regenerated, because two rules sharing an id share a history.
func TestEnsureRuleIDs_RegeneratesDuplicatesAndKeepsTheFirst(t *testing.T) {
	r := Rules{
		TCP: []PortRule{{Port: "22", ID: "abcabcabcabc"}, {Port: "80", ID: "abcabcabcabc"}},
		UDP: []PortRule{{Port: "53", ID: "abcabcabcabc"}},
	}
	if !EnsureRuleIDs(&r) {
		t.Fatal("EnsureRuleIDs reported no change on a set with three rules sharing one id")
	}
	if r.TCP[0].ID != "abcabcabcabc" {
		t.Errorf("the first occurrence lost its id (%q); it is the one with the history", r.TCP[0].ID)
	}
	if r.TCP[1].ID == "abcabcabcabc" || r.UDP[0].ID == "abcabcabcabc" {
		t.Error("a duplicate id survived; the two rules would share one counter")
	}
	if r.TCP[1].ID == r.UDP[0].ID {
		t.Error("the two regenerated ids collided with each other")
	}
}

// omitempty, and it is the same promise Sources made in 2.11: a rules.json
// written before this release is byte-identical until something touches it.
func TestPortRuleWithoutAnIDMarshalsWithoutTheKey(t *testing.T) {
	data, err := json.Marshal(PortRule{Port: "80", Description: "HTTP"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "id") {
		t.Errorf("a rule with no id marshalled as %s; omitempty is missing", data)
	}
}

// The backstop for a rules.json edited by hand. EnsureRuleIDs repairs
// duplicates on every write path, so this should be unreachable in practice —
// which is exactly why it has to be asserted rather than assumed.
func TestValidateRules_RejectsTwoRulesSharingAnID(t *testing.T) {
	err := ValidateRules(Rules{
		TCP: []PortRule{{Port: "22", ID: "abcabcabcabc"}},
		UDP: []PortRule{{Port: "53", ID: "abcabcabcabc"}},
	})
	if err == nil {
		t.Fatal("two rules sharing an id were accepted; they would share one usage counter")
	}
	if !strings.Contains(err.Error(), "abcabcabcabc") {
		t.Errorf("the error does not name the offending id: %v", err)
	}
}

// And an empty id is not a duplicate of another empty id — a pre-2.15 file has
// nothing but those, and refusing to read it would be a migration failure.
func TestValidateRules_AcceptsSeveralRulesWithNoID(t *testing.T) {
	if err := ValidateRules(Rules{
		TCP: []PortRule{{Port: "22"}, {Port: "80"}},
	}); err != nil {
		t.Fatalf("a rule set written before 2.15 was rejected: %v", err)
	}
}
