package web

import (
	"reflect"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// Every verdict the apply screen can reach has a sentence in every strict
// locale. A reason with no key renders as its own message id — "reach_bogon_filter"
// on the page — on the one screen whose whole job is to be believed.
func TestEveryReachReasonHasALabel(t *testing.T) {
	strict := strictLangSet()
	for lang := range strict {
		ids := localeIDs(t, lang)
		for _, reason := range shared.AllReachReasons {
			key := "reach_" + string(reason)
			if !ids[key] {
				t.Errorf("locales/%s.json has no %q for reach verdict reason %q", lang, key, reason)
			}
		}
	}
}

// And every verdict itself.
func TestEveryReachVerdictHasALabel(t *testing.T) {
	strict := strictLangSet()
	for lang := range strict {
		ids := localeIDs(t, lang)
		for _, v := range []shared.ReachVerdict{
			shared.ReachOpen, shared.ReachBlocked, shared.ReachUnknown,
		} {
			key := "apply_verdict_" + string(v)
			if !ids[key] {
				t.Errorf("locales/%s.json has no %q", lang, key)
			}
		}
	}
}

// Every catalogue suggestion has a rationale in every strict locale. ports.html
// picks the label with a binary `if eq … "private"`, so a third Suggestion
// constant would silently render as "Anywhere" rather than fail anywhere — this
// is the guard AllSuggestions' doc comment refers to, and it fails loudly on
// the missing label instead.
func TestEverySuggestionHasALabel(t *testing.T) {
	strict := strictLangSet()
	for lang := range strict {
		ids := localeIDs(t, lang)
		for _, s := range shared.AllSuggestions {
			key := "ports_suggest_" + string(s)
			if !ids[key] {
				t.Errorf("locales/%s.json has no %q for catalogue suggestion %q", lang, key, s)
			}
		}
	}
	if len(shared.AllSuggestions) != 2 {
		t.Errorf("AllSuggestions has %d entries; ports.html's binary `if eq … \"private\"` "+
			"only ever renders two labels — a third entry needs the template fixed too, "+
			"not just this list", len(shared.AllSuggestions))
	}
}

// The set headings come from the same list DiffRules labels its deltas with.
func TestEveryPreviewSetHasALabel(t *testing.T) {
	strict := strictLangSet()
	for lang := range strict {
		ids := localeIDs(t, lang)
		for _, set := range append(previewSetOrder, "options") {
			key := "apply_set_" + set
			if !ids[key] {
				t.Errorf("locales/%s.json has no %q", lang, key)
			}
		}
	}
	// Derived from shared.Rules itself, not a hand-written 6: that literal stayed
	// satisfied the moment a seventh field was added to the struct, while
	// DiffRules — which reflects over the same struct — went on reporting
	// deltas for it that the page never draws, silently, by exactly one field.
	if want := reflect.TypeOf(shared.Rules{}).NumField(); len(previewSetOrder) != want {
		t.Errorf("previewSetOrder holds %d sets; shared.Rules has %d fields, and a set "+
			"missing from this list is a section the preview never draws",
			len(previewSetOrder), want)
	}
}
