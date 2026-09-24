package web

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jp1337/easywall/internal/shared"
)

// The fixtures reproduce each format's shape as the research downloaded it on
// 2026-09-24, with documentation addresses (RFC 5737, RFC 3849) instead of the
// real ones: Spamhaus's terms forbid republishing the list, and a test file of
// other people's addresses is a list published from this repository.

// A plain list, with every trap the research found in one: blocklist.de's
// v4/v6 mix, IPsum-style header comments, CRLF, an inline comment, CrowdSec's
// aggregated CIDR lines, an address written ::ffff:, Hagezi's 240/4 entry
// (kept here; the core strips it), a hosts-file line, a zone.
const plainFixture = "# IPsum Threat Intelligence Feed\r\n" +
	"#\r\n" +
	"192.0.2.10\r\n" +
	"192.0.2.10\r\n" + // duplicate
	"; a semicolon comment\n" +
	"\n" +
	"198.51.100.7    # inline comment\n" +
	"::ffff:198.51.100.8\n" + // unmapped → 198.51.100.8
	"198.51.100.9/32\n" + // full length → bare
	"203.0.113.77/24\n" + // unmasked CIDR → 203.0.113.0/24
	"203.0.113.0/24\n" + // the same network again
	"2001:db8::1\n" +
	"2001:db8:0:0::2/128\n" + // → 2001:db8::2
	"2001:db8:1::/48\n" +
	"245.185.47.110\n" + // 240/4, Hagezi TIF carries three
	"0.0.0.0 example.com\n" + // a hosts file is not a list
	"fe80::1%eth0\n" + // a zone names the publisher's interface
	"not an address\n"

func TestPlainFeedIsCanonicalDedupedAndCountsWhatItRefuses(t *testing.T) {
	entries, rejected, sample, err := parseFeed(shared.FeedFormatPlain, []byte(plainFixture))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"192.0.2.10", "198.51.100.7", "198.51.100.8", "198.51.100.9",
		"203.0.113.0/24", "245.185.47.110",
		"2001:db8::1", "2001:db8::2", "2001:db8:1::/48",
	}
	if !slices.Equal(entries, want) {
		t.Errorf("entries\n got %q\nwant %q", entries, want)
	}
	if rejected != 3 {
		t.Errorf("rejected %d lines, want 3 (hosts line, zone, garbage)", rejected)
	}
	wantSample := []string{"0.0.0.0 example.com", "fe80::1%eth0", "not an address"}
	if !slices.Equal(sample, wantSample) {
		t.Errorf("sample %q, want %q", sample, wantSample)
	}
}

// dan.me.uk answers a fast poller with 200 and an apology. Zero entries: the
// refresh then fails as "empty" instead of loading nothing.
func TestAnErrorSentenceIsNotAList(t *testing.T) {
	body := "Umm... You can only fetch the data every 30 minutes - sorry.  It's pointless any faster as I only update every 30 minutes anyway.\n" +
		"If you keep trying to download this list too often, you may get blocked from accessing it completely.\n"
	entries, rejected, sample, err := parseFeed(shared.FeedFormatPlain, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 || rejected != 2 || len(sample) != 2 {
		t.Fatalf("entries %q, rejected %d, sample %q: want none, 2, 2", entries, rejected, sample)
	}
	if len(sample[0]) > feedSampleBytes || !strings.HasPrefix(sample[0], "Umm... You can only fetch") {
		t.Errorf("sample[0] = %q (%d bytes): want the sentence cut to %d bytes", sample[0], len(sample[0]), feedSampleBytes)
	}
}

func TestSpamhausNDJSONSkipsTheMetadataRecord(t *testing.T) {
	body := `{"cidr":"192.0.2.0/24","sblid":"SBL000001","rir":"arin"}
{"cidr":"198.51.100.0/22","sblid":"SBL000002","rir":"ripencc"}
{"cidr":"2001:db8::/32","sblid":"SBL000003","rir":"ripencc"}
{"cidr":"not-a-prefix","sblid":"SBL000004","rir":"apnic"}
{"cidr":
{"type":"metadata","timestamp":1790257442,"size":104518,"records":3,"copyright":"(c) 2026 The Spamhaus Project SLU","terms":"https://www.spamhaus.org/drop/terms/"}
`
	entries, rejected, _, err := parseFeed(shared.FeedFormatSpamhaus, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"192.0.2.0/24", "198.51.100.0/22", "2001:db8::/32"}
	if !slices.Equal(entries, want) {
		t.Errorf("entries %q, want %q", entries, want)
	}
	if rejected != 2 {
		t.Errorf("rejected %d, want 2: the bad cidr and the broken line — never the metadata record", rejected)
	}
}

func TestDShieldBuildsStartSlashPrefixlen(t *testing.T) {
	body := "#\n#   DShield.org Recommended Block List \n#    Columns (tab delimited):\n#\n" +
		"192.0.2.0\t192.0.2.255\t24\t353\tEXAMPLE-AS\tHK\tabuse@example.net\n" +
		"198.51.100.0\t198.51.100.255\t24\t346\t-\t-\t-\n" +
		"203.0.113.5\t203.0.113.255\t24\t1\t-\t-\t-\n" + // start is not the network
		"Start\tEnd\tNetblock\tAttacks\tName\tCountry\temail\n" + // a header row, if one appears
		"198.51.100.0\t198.51.100.255\n" // too few columns
	entries, rejected, _, err := parseFeed(shared.FeedFormatDShield, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"192.0.2.0/24", "198.51.100.0/24"}; !slices.Equal(entries, want) {
		t.Errorf("entries %q, want %q", entries, want)
	}
	if rejected != 3 {
		t.Errorf("rejected %d, want 3", rejected)
	}
}

func TestTheSampleIsFiveLinesOfAtMostEightyBytes(t *testing.T) {
	long := strings.Repeat("x", 79) + "ä" + strings.Repeat("y", 100) // the cut falls inside "ä"
	body := long + "\nbell\x07here\n" + "a\nb\nc\nd\ne\n"
	_, rejected, sample, err := parseFeed(shared.FeedFormatPlain, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if rejected != 7 || len(sample) != feedSampleLines {
		t.Fatalf("rejected %d, sample %d lines: want 7 and %d", rejected, len(sample), feedSampleLines)
	}
	if len(sample[0]) > feedSampleBytes || !utf8.ValidString(sample[0]) {
		t.Errorf("sample[0] is %d bytes, valid UTF-8 %v: want ≤ %d and valid", len(sample[0]), utf8.ValidString(sample[0]), feedSampleBytes)
	}
	if sample[1] != "bell?here" {
		t.Errorf("sample[1] = %q, want the control character shown as ?", sample[1])
	}
}

func TestAnUnknownFormatIsAnError(t *testing.T) {
	if _, _, _, err := parseFeed("csv", []byte("192.0.2.1\n")); err == nil {
		t.Fatal("an unknown format parsed")
	}
}

// Every catalogue feed's format is one parseFeed reads: refresh discards the
// format error because it cannot happen, and this is why.
func TestEveryCatalogueFormatIsParsed(t *testing.T) {
	for _, f := range shared.FeedCatalogue {
		if _, _, _, err := parseFeed(f.Format, nil); err != nil {
			t.Errorf("%s: %v", f.ID, err)
		}
	}
}
