package web

import (
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// Spec §1: the verdict, the false-positive level and its why are catalogue
// fields rendered beside the switch, not prose that can drift from the data —
// so every catalogue feed carries all three in both strict languages, and so do
// the two link labels every row shows (plan P17). The interface asks for these
// through computed keys (FeedLocaleKey, feed_verdict_<v>, feed_fp_<level>),
// which TestTemplatesOnlyUseTranslatedKeys cannot see.
func TestEveryCatalogueFeedSaysWhatItCosts(t *testing.T) {
	for _, lang := range StrictLangs {
		strs := localeStrings(t, lang)
		need := func(id, what string) {
			t.Helper()
			text, ok := strs[id]
			switch {
			case !ok:
				t.Errorf("locales/%s.json has no %q (%s)", lang, id, what)
			case strings.TrimSpace(text) == "":
				t.Errorf("locales/%s.json has an empty %q (%s)", lang, id, what)
			case strings.ContainsAny(text, "`*") || strings.Contains(text, "{}"):
				t.Errorf("locales/%s.json %q carries markup; a feed row renders it with plain T: %q", lang, id, text)
			}
		}
		for _, f := range shared.FeedCatalogue {
			need("feed_verdict_"+string(f.Verdict), f.ID+"'s verdict")
			need("feed_fp_"+string(f.FalsePositives), f.ID+"'s false-positive level")
			need(shared.FeedLocaleKey(f.ID, "blocks"), "what "+f.ID+" blocks")
			need(shared.FeedLocaleKey(f.ID, "fp_why"), "why "+f.ID+"'s false positives are what they are")
		}
		for _, v := range shared.AllFeedVerdicts {
			need("feed_verdict_"+string(v), "a verdict")
		}
		for _, fp := range shared.AllFeedFalsePositives {
			need("feed_fp_"+string(fp), "a false-positive level")
		}
		need("feed_link_source", "the link to a list's homepage")
		need("feed_link_terms", "the link to a list's terms")
	}
}
