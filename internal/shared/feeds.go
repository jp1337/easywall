package shared

import (
	"strconv"
	"strings"
	"time"
)

// The feed catalogue: other people's lists, each switched on individually,
// all off by default (2.23, spec §1).
//
// Go data for the reason catalogue.go gives: a table the compiler checks. The
// core uses the ids only — which ids exist, and nothing else. The URLs,
// formats and intervals are the web process's, which fetches and parses; the
// verdict and the false-positive level are what the interface renders beside
// the switch, and their sentences live in the locale files under
// FeedLocaleKey. TestEveryCatalogueFeedSaysWhatItCosts holds both locales to
// every row.

// FeedFormat is how a list's body is written.
type FeedFormat string

const (
	// FeedFormatPlain is one address or prefix per line, '#' and ';' comments.
	FeedFormatPlain FeedFormat = "plain"
	// FeedFormatSpamhaus is {"cidr":…} per line and a trailing
	// {"type":"metadata",…} record.
	FeedFormatSpamhaus FeedFormat = "spamhaus-ndjson"
	// FeedFormatDShield is start<TAB>end<TAB>prefixlen…, '#' comments.
	FeedFormatDShield FeedFormat = "dshield"
)

// FeedVerdict is the catalogue's recommendation, rendered ✓ • ✗. The three
// marks follow firebog's: least likely to lock out a legitimate client, useful
// with a stated cost, a deliberate choice rather than a default.
type FeedVerdict string

const (
	FeedRecommended FeedVerdict = "recommended" // ✓
	FeedOptional    FeedVerdict = "optional"    // •
	FeedDeliberate  FeedVerdict = "deliberate"  // ✗ — confirm before staging
)

// AllFeedVerdicts is every verdict; each has a label feed_verdict_<v>.
var AllFeedVerdicts = []FeedVerdict{FeedRecommended, FeedOptional, FeedDeliberate}

// FeedFalsePositives is how likely a list is to refuse a legitimate client:
// a visitor, a webhook sender, a mail server, your own monitoring.
type FeedFalsePositives string

const (
	FalsePositivesNone     FeedFalsePositives = "none" // "practically none"
	FalsePositivesSome     FeedFalsePositives = "some"
	FalsePositivesNotable  FeedFalsePositives = "notable" // "yes, notable"
	FalsePositivesYes      FeedFalsePositives = "yes"
	FalsePositivesByDesign FeedFalsePositives = "by_design"
)

// AllFeedFalsePositives is every level; each has a label feed_fp_<level>.
var AllFeedFalsePositives = []FeedFalsePositives{
	FalsePositivesNone, FalsePositivesSome, FalsePositivesNotable,
	FalsePositivesYes, FalsePositivesByDesign,
}

// CatalogueFeed is one list easywall offers by name.
type CatalogueFeed struct {
	ID   string
	Name string // a proper name; rendered as it is in every language
	// URLs are fetched and merged into one update. Spamhaus publishes v4 and
	// v6 as two files; every other list is one.
	URLs     []string
	Format   FeedFormat
	Interval time.Duration

	Verdict        FeedVerdict
	FalsePositives FeedFalsePositives

	// Homepage and Terms are pages a human reads — never a data URL. Every
	// row links to both beside its verdict (plan P17).
	Homepage string
	Terms    string

	// MeasuredAt is the date of the figures in the reference table. The
	// release that changes a feed's verdict re-measures it.
	MeasuredAt string
}

// FeedCatalogue is spec §1, in its order. Measured 2026-09-24; the sources
// for every URL are in docs-tech/specs/2026-09-24-2.23-research.md.
var FeedCatalogue = []CatalogueFeed{
	{
		ID: "spamhaus-drop", Name: "Spamhaus DROP",
		URLs: []string{
			"https://www.spamhaus.org/drop/drop_v4.json",
			"https://www.spamhaus.org/drop/drop_v6.json",
		},
		Format: FeedFormatSpamhaus, Interval: 12 * time.Hour,
		Verdict: FeedRecommended, FalsePositives: FalsePositivesNone,
		Homepage:   "https://www.spamhaus.org/blocklists/do-not-route-or-peer/",
		Terms:      "https://www.spamhaus.org/drop/terms/",
		MeasuredAt: "2026-09-24",
	},
	{
		ID: "dshield", Name: "DShield top block list",
		URLs:   []string{"https://feeds.dshield.org/block.txt"},
		Format: FeedFormatDShield, Interval: time.Hour,
		Verdict: FeedRecommended, FalsePositives: FalsePositivesNone,
		Homepage:   "https://isc.sans.edu/api/",
		Terms:      "https://isc.sans.edu/api/",
		MeasuredAt: "2026-09-24",
	},
	{
		ID: "blocklist-de", Name: "blocklist.de (all)",
		URLs:   []string{"https://lists.blocklist.de/lists/all.txt"},
		Format: FeedFormatPlain, Interval: time.Hour,
		Verdict: FeedRecommended, FalsePositives: FalsePositivesSome,
		Homepage:   "https://www.blocklist.de/en/export.html",
		Terms:      "https://www.blocklist.de/en/export.html",
		MeasuredAt: "2026-09-24",
	},
	{
		ID: "cins", Name: "CINS Army",
		URLs:   []string{"https://cinsscore.com/list/ci-badguys.txt"},
		Format: FeedFormatPlain, Interval: time.Hour,
		Verdict: FeedOptional, FalsePositives: FalsePositivesSome,
		Homepage:   "https://cinsscore.com/",
		Terms:      "https://cinsscore.com/",
		MeasuredAt: "2026-09-24",
	},
	{
		ID: "et-compromised", Name: "Emerging Threats compromised",
		URLs:   []string{"https://rules.emergingthreats.net/blockrules/compromised-ips.txt"},
		Format: FeedFormatPlain, Interval: time.Hour,
		Verdict: FeedOptional, FalsePositives: FalsePositivesNotable,
		Homepage:   "https://rules.emergingthreats.net/OPEN_download_instructions.html",
		Terms:      "https://rules.emergingthreats.net/open/suricata-7.0.3/LICENSE",
		MeasuredAt: "2026-09-24",
	},
	{
		ID: "ipsum-3", Name: "IPsum, on ≥ 3 lists",
		URLs:   []string{"https://raw.githubusercontent.com/stamparm/ipsum/master/levels/3.txt"},
		Format: FeedFormatPlain, Interval: 24 * time.Hour,
		Verdict: FeedDeliberate, FalsePositives: FalsePositivesYes,
		Homepage:   "https://github.com/stamparm/ipsum",
		Terms:      "https://github.com/stamparm/ipsum",
		MeasuredAt: "2026-09-24",
	},
	{
		ID: "hagezi-tif", Name: "Hagezi threat intelligence IPs",
		URLs:   []string{"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/ips/tif.txt"},
		Format: FeedFormatPlain, Interval: 24 * time.Hour,
		Verdict: FeedDeliberate, FalsePositives: FalsePositivesYes,
		Homepage:   "https://github.com/hagezi/dns-blocklists",
		Terms:      "https://github.com/hagezi/dns-blocklists",
		MeasuredAt: "2026-09-24",
	},
	{
		ID: "tor-exits", Name: "Tor exit nodes",
		URLs:   []string{"https://check.torproject.org/torbulkexitlist"},
		Format: FeedFormatPlain, Interval: time.Hour,
		Verdict: FeedDeliberate, FalsePositives: FalsePositivesByDesign,
		Homepage:   "https://metrics.torproject.org/collector.html",
		Terms:      "https://metrics.torproject.org/collector.html",
		MeasuredAt: "2026-09-24",
	},
}

// The limits both processes enforce on a feed. The core re-checks every one;
// the web process checks them first only to fail a refresh early.
const (
	// MaxOwnFeeds is how many lists an operator may add by URL (spec D8).
	MaxOwnFeeds = 3
	// OwnFeedInterval: CrowdSec's free tier allows one pull a day.
	OwnFeedInterval = 24 * time.Hour
	// FeedMaxEntries bounds one feed's copy. Hagezi TIF, the largest offered,
	// is 79 175.
	FeedMaxEntries = 100000
	// FeedShrinkPercent: a version with fewer than 70 % of the stored count is
	// refused (spec D5 — shrink only; CINS rotates 13.9 % at constant size).
	FeedShrinkPercent = 70
	// FeedMaxBitsV4 and FeedMaxBitsV6: a prefix broader than /8 or /16 refuses
	// the whole update (spec D6; the broadest offered today is /12 and /29).
	FeedMaxBitsV4 = 8
	FeedMaxBitsV6 = 16
)

const ownFeedPrefix = "own-"

// OwnFeedID is the id of own feed n, 1..MaxOwnFeeds: "own-1".."own-3".
func OwnFeedID(n int) string { return ownFeedPrefix + strconv.Itoa(n) }

// IsOwnFeedID reports whether id is one of the MaxOwnFeeds own-feed ids, and
// exactly that spelling: "own-01" and "own-4" are not.
func IsOwnFeedID(id string) bool {
	for n := 1; n <= MaxOwnFeeds; n++ {
		if id == OwnFeedID(n) {
			return true
		}
	}
	return false
}

// CatalogueFeedByID finds a catalogue row.
func CatalogueFeedByID(id string) (CatalogueFeed, bool) {
	for _, f := range FeedCatalogue {
		if f.ID == id {
			return f, true
		}
	}
	return CatalogueFeed{}, false
}

// KnownFeedID reports whether id may be switched on: a catalogue id or an
// own-feed id. It is the whole of what the core knows about the catalogue.
func KnownFeedID(id string) bool {
	_, ok := CatalogueFeedByID(id)
	return ok || IsOwnFeedID(id)
}

// FeedLocaleKey is the message id of one of a catalogue feed's sentences:
// ("spamhaus-drop", "blocks") → "feed_spamhaus_drop_blocks". Fields are
// "blocks" (what it blocks) and "fp_why" (why its false-positive level is what
// it is).
func FeedLocaleKey(id, field string) string {
	return "feed_" + strings.ReplaceAll(id, "-", "_") + "_" + field
}

// FeedDisplayName is what a feed id is called where a person reads it: the
// catalogue name, "Own feed N", or the bare id for an id nothing knows.
func FeedDisplayName(id string) string {
	if f, ok := CatalogueFeedByID(id); ok {
		return f.Name
	}
	if IsOwnFeedID(id) {
		return "Own feed " + strings.TrimPrefix(id, ownFeedPrefix)
	}
	return id
}
