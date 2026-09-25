package shared

import (
	"net/url"
	"regexp"
	"slices"
	"testing"
	"time"
)

// The catalogue is data other code trusts without looking: the core only asks
// whether an id exists, the web process fetches whatever URL a row names and
// parses it by the row's format, and the interface renders the verdict. So
// every row is checked for shape here, once.
func TestFeedCatalogueRowsAreComplete(t *testing.T) {
	if len(FeedCatalogue) != 8 {
		t.Fatalf("the catalogue has %d rows; spec §1 offers eight", len(FeedCatalogue))
	}
	idRe := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	seen := map[string]bool{}
	for _, f := range FeedCatalogue {
		if !idRe.MatchString(f.ID) || IsOwnFeedID(f.ID) {
			t.Errorf("%q is not a catalogue id: lowercase words joined by '-', and never own-N", f.ID)
		}
		if seen[f.ID] {
			t.Errorf("%q is in the catalogue twice", f.ID)
		}
		seen[f.ID] = true
		if f.Name == "" || len(f.URLs) == 0 || f.MeasuredAt != "2026-09-24" {
			t.Errorf("%s: name %q, %d URLs, measured %q", f.ID, f.Name, len(f.URLs), f.MeasuredAt)
		}
		if f.Interval < time.Hour {
			t.Errorf("%s polls every %s; no offered list may be asked more often than hourly", f.ID, f.Interval)
		}
		for _, u := range f.URLs {
			if p, err := url.Parse(u); err != nil || p.Scheme != "https" || p.Host == "" {
				t.Errorf("%s: data URL %q is not https", f.ID, u)
			}
		}
		if !slices.Contains([]FeedFormat{FeedFormatPlain, FeedFormatSpamhaus, FeedFormatDShield}, f.Format) {
			t.Errorf("%s: format %q has no parser", f.ID, f.Format)
		}
		if !slices.Contains(AllFeedVerdicts, f.Verdict) {
			t.Errorf("%s: verdict %q is not one of %v", f.ID, f.Verdict, AllFeedVerdicts)
		}
		if !slices.Contains(AllFeedFalsePositives, f.FalsePositives) {
			t.Errorf("%s: false-positive level %q is not one of %v", f.ID, f.FalsePositives, AllFeedFalsePositives)
		}
	}
}

// Plan P17: every row links to where the list comes from and to its terms. A
// page a human reads — the data URL would download 79 175 addresses into the
// operator's browser, and it says nothing about who may use it.
func TestEveryCatalogueFeedLinksToItsSourceAndTerms(t *testing.T) {
	for _, f := range FeedCatalogue {
		for _, link := range []struct{ what, u string }{{"Homepage", f.Homepage}, {"Terms", f.Terms}} {
			p, err := url.Parse(link.u)
			if err != nil || p.Scheme != "https" || p.Host == "" {
				t.Errorf("%s: %s %q is not an https URL", f.ID, link.what, link.u)
			}
			if slices.Contains(f.URLs, link.u) {
				t.Errorf("%s: %s is the data URL %q, not a page a person reads", f.ID, link.what, link.u)
			}
		}
	}
}

func TestFeedIDs(t *testing.T) {
	for _, tc := range []struct {
		id         string
		own, known bool
	}{
		{"own-1", true, true}, {"own-3", true, true},
		{"own-0", false, false}, {"own-4", false, false}, {"own-01", false, false}, {"own-", false, false},
		{"spamhaus-drop", false, true}, {"tor-exits", false, true},
		{"Spamhaus-Drop", false, false}, {"firehol-level1", false, false}, {"", false, false},
	} {
		if got := IsOwnFeedID(tc.id); got != tc.own {
			t.Errorf("IsOwnFeedID(%q) = %v, want %v", tc.id, got, tc.own)
		}
		if got := KnownFeedID(tc.id); got != tc.known {
			t.Errorf("KnownFeedID(%q) = %v, want %v", tc.id, got, tc.known)
		}
	}
	if OwnFeedID(2) != "own-2" {
		t.Errorf("OwnFeedID(2) = %q", OwnFeedID(2))
	}
	if f, ok := CatalogueFeedByID("dshield"); !ok || f.Format != FeedFormatDShield {
		t.Errorf("CatalogueFeedByID(dshield) = %+v, %v", f, ok)
	}
	if got := FeedLocaleKey("spamhaus-drop", "blocks"); got != "feed_spamhaus_drop_blocks" {
		t.Errorf("FeedLocaleKey = %q", got)
	}
	for id, want := range map[string]string{"spamhaus-drop": "Spamhaus DROP", "own-2": "Own feed 2", "nope": "nope"} {
		if got := FeedDisplayName(id); got != want {
			t.Errorf("FeedDisplayName(%q) = %q, want %q", id, got, want)
		}
	}
}

// Spec §1, row by row. The verdict decides whether staging a feed asks for the
// ✗ confirmation, so a row moved from ✗ to • — or • to ✓ — is a feed switched
// on without the operator being told whom it locks out.
func TestFeedCatalogueMatchesSpecOne(t *testing.T) {
	want := map[string]struct {
		v        FeedVerdict
		fp       FeedFalsePositives
		interval time.Duration
	}{
		"spamhaus-drop":  {FeedRecommended, FalsePositivesNone, 12 * time.Hour},
		"dshield":        {FeedRecommended, FalsePositivesNone, time.Hour},
		"blocklist-de":   {FeedRecommended, FalsePositivesSome, time.Hour},
		"cins":           {FeedOptional, FalsePositivesSome, time.Hour},
		"et-compromised": {FeedOptional, FalsePositivesNotable, time.Hour},
		"ipsum-3":        {FeedDeliberate, FalsePositivesYes, 24 * time.Hour},
		"hagezi-tif":     {FeedDeliberate, FalsePositivesYes, 24 * time.Hour},
		"tor-exits":      {FeedDeliberate, FalsePositivesByDesign, time.Hour},
	}
	if len(FeedCatalogue) != len(want) {
		t.Fatalf("%d rows, spec §1 has %d", len(FeedCatalogue), len(want))
	}
	for _, f := range FeedCatalogue {
		w, ok := want[f.ID]
		if !ok {
			t.Errorf("%s is not in spec §1", f.ID)
			continue
		}
		if f.Verdict != w.v || f.FalsePositives != w.fp || f.Interval != w.interval {
			t.Errorf("%s: verdict %s, false positives %s, every %s; spec §1 says %s, %s, %s",
				f.ID, f.Verdict, f.FalsePositives, f.Interval, w.v, w.fp, w.interval)
		}
	}
}
