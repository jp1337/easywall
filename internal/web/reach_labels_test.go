package web

import (
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
	if len(previewSetOrder) != 6 {
		t.Errorf("previewSetOrder holds %d sets; shared.Rules has six, and a set "+
			"missing from this list is a section the preview never draws",
			len(previewSetOrder))
	}
}
