package core

import (
	"slices"
	"testing"

	"github.com/google/nftables/expr"
	"github.com/jp1337/easywall/internal/shared"
)

// 2.26: the United States' aggregated list holds 16.0.0.0/6, 32.0.0.0/6 and
// 2630::/16 (measured 2026-10-13). A country takes IPv4 to /4; a list still
// takes /8, so the same prefix refuses a list's whole update. IPv6 is /16 for
// both: no country's is broader.
func TestValidateFeedEntries_ACountryMayBeBroader(t *testing.T) {
	us := []string{"16.0.0.0/6", "32.0.0.0/6", "214.0.0.0/7", "2630::/16", "48.0.0.0/4"}
	kept, _, err := validateFeedEntries("country-us", us)
	if err != nil || len(kept) != len(us) {
		t.Fatalf("a country's /4 to /7 and /16: kept %d of %d, %v", len(kept), len(us), err)
	}
	for _, e := range []string{"32.0.0.0/3", "2000::/15"} {
		if _, _, err := validateFeedEntries("country-us", []string{e}); err == nil {
			t.Errorf("a country's %s was accepted", e)
		}
	}
	if _, _, err := validateFeedEntries("dshield", []string{"16.0.0.0/6"}); err == nil {
		t.Error("a list's /6 was accepted")
	}
}

// Through UPDATE_FEED: the limit is the payload's id's, not the lists'.
func TestUpdateFeed_StoresACountryWithItsBroadPrefixes(t *testing.T) {
	fw := feedFirewall(t, "country-us")
	res, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: "country-us",
		Entries: []string{"16.0.0.0/6", "2630::/16"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if res.After != 2 || !res.Changed {
		t.Errorf("result %+v", res)
	}
	if rec, ok, _ := fw.feeds.Get("country-us"); !ok || !slices.Equal(rec.Prefixes, []string{"16.0.0.0/6", "2630::/16"}) {
		t.Errorf("stored %+v, %v", rec, ok)
	}
}

// Spec D8: a country's rules follow every list's, whatever order they were
// switched on in, so an address on both is counted and logged by the list.
func TestAddFeedsChecksCountriesAfterTheLists(t *testing.T) {
	c, _ := captureConn(t)
	rec := &recordingConn{}
	m := &NftablesManager{conn: c, adder: rec, logSink: logSink{nflog: true, group: 7}}
	tbl := easywallInetTableForTest()
	if _, err := m.addFeeds(tbl, inputChainForTest(tbl), []string{"country-de", "dshield", "country-cn", "own-1"},
		FeedContents{}, shared.FirewallOptions{}); err != nil {
		t.Fatal(err)
	}
	var sets []string
	for _, r := range rec.rules {
		for _, e := range r.Exprs {
			if l, ok := e.(*expr.Lookup); ok && !slices.Contains(sets, l.SetName) {
				sets = append(sets, l.SetName)
			}
		}
	}
	want := []string{"feed-dshield-v4", "feed-dshield-v6", "feed-own-1-v4", "feed-own-1-v6",
		"feed-country-de-v4", "feed-country-de-v6", "feed-country-cn-v4", "feed-country-cn-v6"}
	if !slices.Equal(sets, want) {
		t.Errorf("rules look up %v, want %v", sets, want)
	}
}
