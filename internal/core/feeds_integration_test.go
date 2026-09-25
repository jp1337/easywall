//go:build integration

package core

import (
	"fmt"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// A refresh swaps a set's contents and leaves the rule — and its counter — as
// they were. So GET_FEEDS's packet count is "since the last apply", as the
// card says, and UPDATE_FEED has no counters to book before it writes.
func TestIntegration_ARefreshKeepsTheCounter(t *testing.T) {
	for _, bin := range []string{"bash", "timeout"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("skipping: %s is not installed", bin)
		}
	}
	m := newIntegrationManager(t)
	r := newRouter(t)
	listenOn(t, feedTestPort)
	rules := shared.Rules{
		TCP:   []shared.PortRule{{Port: strconv.Itoa(feedTestPort), Description: "feed test", ID: "fee0d0000001"}},
		Feeds: []string{"cins"},
	}
	if err := m.ApplyWithFeeds(shared.RulesState{Current: rules}, shared.FirewallOptions{}, shared.NetworkSettings{},
		FeedContents{"cins": {netip.MustParsePrefix("10.77.1.2/32")}}); err != nil {
		t.Fatal(err)
	}
	if tcpReaches(t, r.pidA, "10.77.1.1", feedTestPort) {
		t.Fatal("the source in the feed reached the port")
	}
	before, err := m.RuleCounters()
	if err != nil {
		t.Fatal(err)
	}
	if before[feedCounterID("cins")].Packets == 0 {
		t.Fatalf("the drop counted nothing: %v", before)
	}

	v4, v6 := splitFamilies(mergeRanges(prefixes("198.51.100.0/24", "2001:db8::/32")))
	if err := m.ReplaceFeedSet("cins", v4, v6); err != nil {
		t.Fatal(err)
	}
	after, err := m.RuleCounters()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := after[feedCounterID("cins")].Packets, before[feedCounterID("cins")].Packets; got != want {
		t.Errorf("the refresh moved the counter from %d to %d; a set flush must leave the rule alone", want, got)
	}
	if !tcpReaches(t, r.pidA, "10.77.1.1", feedTestPort) {
		t.Error("the source left the feed and is still dropped: the old contents are in the set")
	}
	if !nftGetElement(t, "feed-cins-v6", "2001:db8::1") {
		t.Error("the new IPv6 contents are not in the set")
	}
}

// Spec §3 step 8: FlushSet and the new elements are one batch, which the kernel
// commits as one transaction. An observer on its own socket asks, as fast as it
// can, for an address that is in both the old and the new contents, while the
// set is replaced five times with 100 000 entries: it must always be there.
// Split into two flushes, the set is empty for as long as the kernel takes to
// load 100 000 elements, and the observer finds it missing.
func TestIntegration_ARefreshNeverEmptiesTheSet(t *testing.T) {
	m := newIntegrationManager(t)
	big := syntheticFeed(shared.FeedMaxEntries, false, 1)
	always := netip.MustParsePrefix("198.51.100.7/32")
	if err := m.ApplyWithFeeds(shared.RulesState{Current: shared.Rules{Feeds: []string{"hagezi-tif"}}},
		shared.FirewallOptions{}, shared.NetworkSettings{}, FeedContents{"hagezi-tif": append(big, always)}); err != nil {
		t.Fatal(err)
	}

	if !setHolds(t, "feed-hagezi-tif-v4", always.Addr()) || setHolds(t, "feed-hagezi-tif-v4", netip.MustParseAddr("198.51.100.8")) {
		t.Fatal("the observer cannot tell a member from a non-member")
	}
	stop := make(chan struct{})
	result := make(chan [2]int)
	go func() {
		looks, missing := 0, 0
		for {
			select {
			case <-stop:
				result <- [2]int{looks, missing}
				return
			default:
			}
			looks++
			if !setHolds(t, "feed-hagezi-tif-v4", always.Addr()) {
				missing++
			}
		}
	}()
	for i := 0; i < 5; i++ {
		v4, v6 := splitFamilies(mergeRanges(append(syntheticFeed(shared.FeedMaxEntries, false, uint32(i+2)<<21), always)))
		if err := m.ReplaceFeedSet("hagezi-tif", v4, v6); err != nil {
			close(stop)
			<-result
			t.Fatal(err)
		}
	}
	close(stop)
	got := <-result
	fmt.Printf("REFRESH observer looked %d times, the address was missing %d times\n", got[0], got[1])
	if got[1] > 0 {
		t.Errorf("an address in both versions was missing from the set %d times in %d looks: "+
			"the flush and the new elements are not one transaction", got[1], got[0])
	}
}

// ReplaceFeedSet adds no table, chain or rule. After panic mode's teardown —
// Reset(), which leaves an empty table — it fails, and no set or chain comes
// back: which is why it needs no panic re-check after it.
func TestIntegration_ARefreshCannotRecreateATornDownTable(t *testing.T) {
	m := newIntegrationManager(t)
	if err := m.ApplyWithFeeds(shared.RulesState{Current: shared.Rules{Feeds: []string{"dshield"}}},
		shared.FirewallOptions{}, shared.NetworkSettings{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := m.Reset(); err != nil {
		t.Fatal(err)
	}
	v4, _ := splitFamilies(mergeRanges(prefixes("198.51.100.0/24")))
	if err := m.ReplaceFeedSet("dshield", v4, nil); err == nil {
		t.Error("a refresh of a set in a torn-down table succeeded")
	}
	out, err := exec.Command("nft", "list", "table", "inet", tableName).CombinedOutput()
	if err != nil {
		t.Fatalf("nft list table: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "set ") || strings.Contains(string(out), "chain ") {
		t.Errorf("the refresh brought something back:\n%s", out)
	}
}

// Spec §3 steps 7–8 against a kernel: an update of a feed in Current is
// stored and swapped into the live set; a refused one leaves the kernel's set
// and the file as they were.
func TestIntegration_UpdateFeedLoadsALiveFeed(t *testing.T) {
	fw := newTestFirewallWithRealNft(t)
	fw.feeds = NewFeedStore(fw.cfg.FeedsPath())
	if err := fw.rules.SaveStaged("feeds", []string{"dshield"}); err != nil {
		t.Fatal(err)
	}
	if err := fw.rules.PromoteStaged(); err != nil {
		t.Fatal(err)
	}
	state, _ := fw.rules.GetState()
	if err := fw.nft.ApplyWithFeeds(state, shared.FirewallOptions{}, shared.NetworkSettings{}, nil); err != nil {
		t.Fatal(err)
	}

	res, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: "dshield", Entries: hosts(20)}, "test")
	if err != nil || !res.Loaded || res.After != 20 {
		t.Fatalf("UpdateFeed = %+v, %v", res, err)
	}
	if !setHolds(t, "feed-dshield-v4", netip.MustParseAddr(hosts(20)[19])) {
		t.Fatal("the stored copy is not in the live set")
	}

	if _, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: "dshield", Entries: hosts(2)}, "test"); err == nil {
		t.Fatal("a copy of 2 after 20 was accepted")
	}
	if !setHolds(t, "feed-dshield-v4", netip.MustParseAddr(hosts(20)[19])) {
		t.Error("a refused update emptied the live set")
	}
	feeds, err := fw.Feeds("")
	if err != nil || len(feeds.Feeds) != 1 || !feeds.Feeds[0].InKernel || !feeds.Feeds[0].CountersRead ||
		feeds.Feeds[0].Entries != 20 {
		t.Errorf("GET_FEEDS after the refusal: %+v, %v", feeds, err)
	}
}

// Rulings X1 end to end, through Firewall.Apply and a kernel: a live feed that
// shrinks is refused; switched off and applied, then switched on, the smaller
// list is accepted and the next apply loads it into the set.
func TestIntegration_AShrunkFeedLoadsAfterOffApplyOn(t *testing.T) {
	fw := newTestFirewallWithRealNft(t)
	fw.feeds = NewFeedStore(fw.cfg.FeedsPath())
	fw.cfg.Acceptance.Enabled = false // an apply is final here; the window is not what this is about
	apply := func() {
		t.Helper()
		if err := fw.Apply("test"); err != nil {
			t.Fatal(err)
		}
	}
	stage := func(ids ...string) {
		t.Helper()
		if err := fw.rules.SaveStaged("feeds", ids); err != nil {
			t.Fatal(err)
		}
	}
	_ = fw.rules.SaveStaged("tcp", []shared.PortRule{{Port: "22", Description: "ssh"}})
	stage("dshield")
	if _, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: "dshield", Entries: hosts(20)}, "test"); err != nil {
		t.Fatal(err)
	}
	apply()
	small := hosts(30)[20:] // ten other addresses
	if _, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: "dshield", Entries: small}, "test"); err == nil ||
		err.Error() != shared.ErrFeedShrankText {
		t.Fatalf("a live feed shrinking to half: %v", err)
	}
	stage()
	apply()
	stage("dshield")
	if _, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: "dshield", Entries: small}, "test"); err != nil {
		t.Fatalf("the smaller list after off, apply, on: %v", err)
	}
	apply()
	if !setHolds(t, "feed-dshield-v4", netip.MustParseAddr(small[0])) {
		t.Error("the apply did not load the accepted smaller copy")
	}
	if setHolds(t, "feed-dshield-v4", netip.MustParseAddr(hosts(1)[0])) {
		t.Error("the old copy is still in the set")
	}
}
