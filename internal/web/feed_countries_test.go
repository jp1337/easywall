package web

import (
	"slices"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// ipverse's country/<cc>/aggregated.json as served on 2026-10-13, with
// documentation addresses in place of the real ones — the rule
// feed_parse_test.go states for every fixture.
const ipverseFixture = `{
  "country": "Germany",
  "countryCode": "DE",
  "delegationStatus": [ "allocated", "assigned" ],
  "exportMode": "aggregated",
  "prefixes": {
    "ipv4": [ "192.0.2.0/24", "198.51.100.0/22", "203.0.113.7/32" ],
    "ipv6": [ "2001:db8::/32" ]
  }
}`

// KP's shape: six countries publish no IPv6 at all (cf, er, fk, kp, ms, yt
// on 2026-10-13). As separate .txt files that was a 404 on every refresh;
// here it is an empty list.
const ipverseNoIPv6 = `{"country": "North Korea", "countryCode": "KP", "prefixes": {"ipv4": [ "192.0.2.0/24" ], "ipv6": [ ]}}`

// Spec §4: the source's own structure parses to no rejects — the country
// name, the status and the export mode are not lines of a list.
func TestAnIPverseCountryParsesToNoRejects(t *testing.T) {
	entries, rejected, sample, err := parseFeed(shared.FeedFormatIPverse, []byte(ipverseFixture))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"192.0.2.0/24", "198.51.100.0/22", "203.0.113.7", "2001:db8::/32"}; !slices.Equal(entries, want) {
		t.Errorf("entries %q, want %q", entries, want)
	}
	if rejected != 0 || len(sample) != 0 {
		t.Errorf("%d rejected: %q", rejected, sample)
	}
	entries, rejected, _, _ = parseFeed(shared.FeedFormatIPverse, []byte(ipverseNoIPv6))
	if !slices.Equal(entries, []string{"192.0.2.0/24"}) || rejected != 0 {
		t.Errorf("a country with no IPv6: entries %q, rejected %d", entries, rejected)
	}
}

// A body that is not the document — a proxy's notice, a truncated file — is
// rejected line by line and shown on the row, and holds no entry, which the
// refresh then refuses as empty.
func TestABodyThatIsNotIPversesDocumentIsRejected(t *testing.T) {
	entries, rejected, sample, _ := parseFeed(shared.FeedFormatIPverse, []byte("rate limited\n{\"prefixes\": {\"ipv4\": [\n"))
	if len(entries) != 0 || rejected != 2 || len(sample) != 2 || sample[0] != "rate limited" {
		t.Errorf("entries %q, rejected %d, sample %q", entries, rejected, sample)
	}
}

// Spec D2: a country is one URL, so its daily refresh asks with the ETag and
// a 304 costs nothing — the reason the source is aggregated.json and not the
// two .txt files, which refreshFrom never asks conditionally.
func TestACountryIsRefreshedConditionally(t *testing.T) {
	h := newFeedHarness(t, false)
	u := h.list(ipverseFixture)
	src, ok := h.r.source("country-kp")
	if !ok || len(src.urls) != 1 || src.format != shared.FeedFormatIPverse || src.interval != shared.CountryInterval {
		t.Fatalf("source(country-kp) = %+v, %v", src, ok)
	}
	src.urls = []string{u}
	h.r.refreshFrom("country-kp", src, false)
	if sent := h.sent(); len(sent) != 1 || len(sent[0].Entries) != 4 {
		t.Fatalf("first refresh sent %+v", sent)
	}
	h.fc.SetResponse(shared.CmdGetFeeds, successResp(shared.GetFeedsResult{Feeds: []shared.FeedStatus{
		{ID: "country-kp", Stored: true}}}))
	h.r.refreshFrom("country-kp", src, false)
	if inm := h.lastHeader("If-None-Match"); inm != `"v1"` {
		t.Errorf("the second refresh sent If-None-Match %q, want the ETag", inm)
	}
	if sent := h.sent(); len(sent) != 2 || !sent[1].NotModified {
		t.Errorf("second refresh sent %+v, want not_modified", sent)
	}
}
