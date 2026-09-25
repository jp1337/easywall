package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// feedFirewall is a firewall with no kernel whose rules switch on stagedFeeds
// in Staged only, so UpdateFeed stores and never loads.
func feedFirewall(t *testing.T, stagedFeeds ...string) *Firewall {
	t.Helper()
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	if err := fw.rules.SaveStaged("feeds", stagedFeeds); err != nil {
		t.Fatal(err)
	}
	return fw
}

// hosts is n distinct global /32s, as the web process would send them.
// liveFeedFirewall is feedFirewall with live switched on in Current too, and
// a connection that accepts every batch in place of a kernel — so the load
// step succeeds and the Current-only guards apply.
func liveFeedFirewall(t *testing.T, live []string, stagedFeeds ...string) *Firewall {
	t.Helper()
	fw := feedFirewall(t, live...)
	if err := fw.rules.PromoteStaged(); err != nil {
		t.Fatal(err)
	}
	if err := fw.rules.SaveStaged("feeds", stagedFeeds); err != nil {
		t.Fatal(err)
	}
	c, _ := captureConn(t)
	fw.nft = &NftablesManager{conn: c}
	return fw
}

// promote stands in for an accepted apply: Backup = Current, Current = Staged.
func promote(t *testing.T, fw *Firewall) {
	t.Helper()
	if err := fw.rules.BackupCurrent(); err != nil {
		t.Fatal(err)
	}
	if err := fw.rules.PromoteStaged(); err != nil {
		t.Fatal(err)
	}
}

func hosts(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("198.%d.%d.%d", 18+i/65536, i/256%256, i%256)
	}
	return out
}

func readFeedsFile(t *testing.T, fw *Firewall) []byte {
	t.Helper()
	b, err := os.ReadFile(fw.cfg.FeedsPath())
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return b
}

// Spec §3 step 3–5: re-parsed, unmapped, masked, deduplicated, sorted; what is
// not globally routable dropped and counted. 2001:db8::/32 (RFC 3849's IPv6
// documentation range) is dropped like the rest, not kept as a v6 TEST-NET
// would be (review finding, D4-4 revision): unlike IPv4's TEST-NETs, which are
// real if unrouted addresses a feed may legitimately carry, 2001:db8::/32 is
// carved out of global space specifically so nobody ships it as reachable.
func TestValidateFeedEntries(t *testing.T) {
	kept, dropped, err := validateFeedEntries([]string{
		"203.0.113.9", " 198.51.100.0/24 ", "198.51.100.77/24", "::ffff:192.0.2.1", "::ffff:192.0.2.0/120",
		"2001:db8::/32", "10.1.2.3", "100.64.0.1", "127.0.0.1", "169.254.1.1", "0.1.2.3",
		"224.0.0.1", "240.0.0.1", "172.16.0.0/12", "192.168.1.0/24", "100.0.0.0/8",
		"fe80::1", "fd00::/16", "::1", "ff02::1", "203.0.113.9/32",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"192.0.2.0/24", "192.0.2.1/32", "198.51.100.0/24", "203.0.113.9/32"}
	var got []string
	for _, p := range kept {
		got = append(got, p.String())
	}
	if !slices.Equal(got, want) {
		t.Errorf("kept %v, want %v", got, want)
	}
	if dropped != 15 {
		t.Errorf("dropped %d, want 15", dropped)
	}
}

// Review finding 1 (D4-4 revision, spec §3 step 4 before step 5): the
// non-global drop runs before the breadth refusal, so a prefix wholly inside
// non-global space is dropped and counted instead of refusing the whole
// update. FireHOL level1 carries its own bogon block — 224.0.0.0/3, 10.0.0.0/8
// and 100.64.0.0/10 among its entries, each matching (or narrower than) a
// nonGlobalV4 range exactly — alongside real addresses; before this fix every
// fetch of it was refused as "224.0.0.0/3 is broader than /8", nothing kept.
func TestValidateFeedEntries_DropsNonGlobalBeforeRefusingBreadth(t *testing.T) {
	kept, dropped, err := validateFeedEntries([]string{
		"224.0.0.0/3", "10.0.0.0/8", "100.64.0.0/10", "1.2.3.0/24",
	})
	if err != nil {
		t.Fatalf("FireHOL level1's bogon lines refused the whole update: %v", err)
	}
	if dropped != 3 {
		t.Errorf("dropped %d, want 3 (the bogon lines)", dropped)
	}
	var got []string
	for _, p := range kept {
		got = append(got, p.String())
	}
	if want := []string{"1.2.3.0/24"}; !slices.Equal(got, want) {
		t.Errorf("kept %v, want %v", got, want)
	}
}

// A prefix that only overlaps non-global space in part — 0.0.0.0/0 and ::/0
// overlap everything, including it — is not "wholly inside" any single
// non-global range, so it is not silently dropped: it still reaches, and
// fails, the breadth check.
func TestValidateFeedEntries_ZeroRoutesStillRefuseOnBreadth(t *testing.T) {
	for _, e := range []string{"0.0.0.0/0", "::/0"} {
		if _, _, err := validateFeedEntries([]string{e}); err == nil {
			t.Errorf("%s was accepted", e)
		}
	}
}

// Spec D6: the broadest in any offered feed is /12 and /29; anything broader
// than /8 or /16 refuses the whole update — including one the drop would
// otherwise have swallowed, like 0.0.0.0/0.
func TestValidateFeedEntries_RefusesTooBroad(t *testing.T) {
	for _, e := range []string{"0.0.0.0/0", "12.0.0.0/7", "2000::/15", "::/0", "::ffff:0.0.0.0/100"} {
		if _, _, err := validateFeedEntries([]string{"203.0.113.9", e}); err == nil {
			t.Errorf("%s was accepted", e)
		}
	}
	for _, e := range []string{"12.0.0.0/8", "2a00::/16"} {
		if _, _, err := validateFeedEntries([]string{e}); err != nil {
			t.Errorf("%s is exactly the limit and was refused: %v", e, err)
		}
	}
}

func TestValidateFeedEntries_RefusesWhatDoesNotParse(t *testing.T) {
	for _, e := range []string{"", "not-an-address", "198.51.100.0/33", "fe80::1%eth0", "::ffff:1.2.3.4/95", "<html>"} {
		_, _, err := validateFeedEntries([]string{"203.0.113.9", e})
		if err == nil {
			t.Errorf("%q was accepted", e)
			continue
		}
		if e != "" && strings.Contains(err.Error(), e) {
			t.Errorf("the refusal quotes the entry (%v); it reaches the audit log, which takes no text the web process chose", err)
		}
	}
}

func TestValidateFeedEntries_RefusesMoreThanTheLimit(t *testing.T) {
	if _, _, err := validateFeedEntries(hosts(shared.FeedMaxEntries + 1)); err == nil {
		t.Error("100 001 entries were accepted")
	}
	if _, _, err := validateFeedEntries(hosts(shared.FeedMaxEntries)); err != nil {
		t.Errorf("exactly the limit was refused: %v", err)
	}
}

func TestShrinks(t *testing.T) {
	for _, tc := range []struct {
		before, after int
		want          bool
	}{{100, 70, false}, {100, 69, true}, {1801, 1261, false}, {1801, 1260, true}, {0, 0, false}, {20, 14, false}, {20, 13, true}} {
		if got := shrinks(tc.before, tc.after); got != tc.want {
			t.Errorf("shrinks(%d, %d) = %v, want %v", tc.before, tc.after, got, tc.want)
		}
	}
}

// Every refusal leaves the previous version on disk (spec §3), and the ones
// that are about the data are audited. Each case starts from a stored copy of
// 20 addresses of dshield, switched on in Staged.
func TestUpdateFeed_EveryRefusalLeavesThePreviousCopy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(t *testing.T, fw *Firewall)
		p       shared.UpdateFeedPayload
		want    string // a substring of the error
		audited bool
	}{
		{name: "panic engaged", p: shared.UpdateFeedPayload{ID: "dshield", Entries: hosts(21)},
			prepare: func(t *testing.T, fw *Firewall) {
				if err := EngagePanic(fw.cfg.PanicMarkerPath()); err != nil {
					t.Fatal(err)
				}
			}, want: shared.ErrPanicEngagedText},
		{name: "the apply slot is held", p: shared.UpdateFeedPayload{ID: "dshield", Entries: hosts(21)},
			prepare: func(t *testing.T, fw *Firewall) { fw.beginApply() }, want: shared.ErrApplyInProgressText},
		{name: "unknown id", p: shared.UpdateFeedPayload{ID: "firehol-level1", Entries: hosts(21)},
			want: "not a feed id", audited: true},
		{name: "not switched on", p: shared.UpdateFeedPayload{ID: "cins", Entries: hosts(21)},
			want: "not switched on"},
		{name: "more than 100 000", p: shared.UpdateFeedPayload{ID: "dshield", Entries: hosts(shared.FeedMaxEntries + 1)},
			want: "at most", audited: true},
		{name: "only what is not globally routable", p: shared.UpdateFeedPayload{ID: "dshield",
			Entries: []string{"10.0.0.1", "192.168.1.1", "fe80::1"}}, want: "no entry is globally routable", audited: true},
		{name: "broader than /8", p: shared.UpdateFeedPayload{ID: "dshield", Entries: append(hosts(20), "12.0.0.0/7")},
			want: "broader", audited: true},
		{name: "broader than /16", p: shared.UpdateFeedPayload{ID: "dshield", Entries: append(hosts(20), "2a00::/15")},
			want: "broader", audited: true},
		{name: "an entry that does not parse", p: shared.UpdateFeedPayload{ID: "dshield", Entries: append(hosts(20), "<html>")},
			want: "entry 21", audited: true},
		{name: "shrinks below 70 % while live", p: shared.UpdateFeedPayload{ID: "dshield", Entries: hosts(13)},
			want: shared.ErrFeedShrankText, audited: true},
		{name: "the kernel refuses the new copy", p: shared.UpdateFeedPayload{ID: "dshield", Entries: hosts(21)},
			prepare: func(t *testing.T, fw *Firewall) { fw.nft = &NftablesManager{} }, want: "load feed", audited: true},
		{name: "a 304 for a feed with no copy", p: shared.UpdateFeedPayload{ID: "own-1", NotModified: true},
			want: "no stored copy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fw := liveFeedFirewall(t, []string{"dshield"}, "dshield", "own-1")
			if _, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: "dshield", Entries: hosts(20)}, "test"); err != nil {
				t.Fatal(err)
			}
			before := readFeedsFile(t, fw)
			auditBefore := len(auditActions(t, fw.cfg))
			if tc.prepare != nil {
				tc.prepare(t, fw)
			}

			_, err := fw.UpdateFeed(tc.p, "test")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("UpdateFeed = %v, want an error containing %q", err, tc.want)
			}
			if after := readFeedsFile(t, fw); !bytes.Equal(before, after) {
				t.Errorf("a refused update changed feeds.json")
			}
			actions := auditActions(t, fw.cfg)[auditBefore:]
			switch {
			case tc.audited && (len(actions) != 1 || actions[0] != "feed_refused"):
				t.Errorf("audit %v, want one feed_refused", actions)
			case !tc.audited && len(actions) != 0:
				t.Errorf("audit %v, want nothing", actions)
			}
			if log, _ := os.ReadFile(fw.cfg.AuditLogPath()); tc.p.ID == "firehol-level1" && strings.Contains(string(log), tc.p.ID) {
				t.Error("the audit log quotes an id no feed has — text the web process chose")
			}
		})
	}
}

// P7, the exact text: the web process holds the update and resends it when
// the reply is ErrApplyInProgressText and nothing else, and it reaches the web
// as Response.Error through errResp.
func TestUpdateFeed_TheSlotRefusalIsTheExactText(t *testing.T) {
	fw := feedFirewall(t, "dshield")
	fw.beginApply()
	_, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: "dshield", Entries: hosts(1)}, "test")
	if resp := errResp(err); resp.Error != shared.ErrApplyInProgressText {
		t.Errorf("Response.Error = %q, want exactly %q", resp.Error, shared.ErrApplyInProgressText)
	}
}

func TestUpdateFeed_StoresAFirstCopy(t *testing.T) {
	fw := feedFirewall(t, "dshield")
	res, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: "dshield",
		Entries: []string{"198.51.100.0/24", "203.0.113.9", "203.0.113.9/32", "10.0.0.1"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if res != (shared.UpdateFeedResult{Before: 0, After: 2, Dropped: 1, Changed: true, Loaded: false}) {
		t.Errorf("result %+v", res)
	}
	rec, ok, _ := fw.feeds.Get("dshield")
	if !ok || !slices.Equal(rec.Prefixes, []string{"198.51.100.0/24", "203.0.113.9/32"}) ||
		rec.Entries != 2 || rec.Dropped != 1 || rec.ChangedAt.IsZero() || rec.CheckedAt != rec.ChangedAt {
		t.Errorf("stored %+v", rec)
	}
	info, err := os.Stat(fw.cfg.FeedsPath())
	if err != nil || info.Mode().Perm() != 0600 {
		t.Errorf("feeds.json: %v, %v; want 0600", info, err)
	}
	if got := auditActions(t, fw.cfg); !slices.Contains(got, "feed_updated") {
		t.Errorf("audit %v, want feed_updated", got)
	}
}

// The same list again: nothing changed, so checked_at moves, changed_at does
// not, and nothing is audited — an hourly list would otherwise write 24 lines
// a day into a log shown 200 lines at a time.
func TestUpdateFeed_TheSameListAgainOnlyMovesCheckedAt(t *testing.T) {
	fw := feedFirewall(t, "dshield")
	p := shared.UpdateFeedPayload{ID: "dshield", Entries: hosts(20)}
	if _, err := fw.UpdateFeed(p, "test"); err != nil {
		t.Fatal(err)
	}
	first, _, _ := fw.feeds.Get("dshield")
	auditBefore := len(auditActions(t, fw.cfg))

	slices.Reverse(p.Entries)
	res, err := fw.UpdateFeed(p, "test")
	if err != nil || res.Changed || res.Before != 20 || res.After != 20 {
		t.Fatalf("UpdateFeed = %+v, %v", res, err)
	}
	second, _, _ := fw.feeds.Get("dshield")
	if !second.ChangedAt.Equal(first.ChangedAt) || !second.CheckedAt.After(first.CheckedAt) {
		t.Errorf("changed %v → %v, checked %v → %v", first.ChangedAt, second.ChangedAt, first.CheckedAt, second.CheckedAt)
	}
	if n := len(auditActions(t, fw.cfg)); n != auditBefore {
		t.Errorf("an unchanged list wrote %d audit entries", n-auditBefore)
	}
}

// Plan P8: a 304 moves checked_at and nothing else.
func TestUpdateFeed_NotModifiedMovesCheckedAt(t *testing.T) {
	fw := feedFirewall(t, "dshield")
	if _, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: "dshield", Entries: hosts(20)}, "test"); err != nil {
		t.Fatal(err)
	}
	first, _, _ := fw.feeds.Get("dshield")
	res, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: "dshield", NotModified: true, Entries: hosts(1)}, "test")
	if err != nil || res.Changed || res.Before != 20 || res.After != 20 {
		t.Fatalf("UpdateFeed = %+v, %v", res, err)
	}
	second, _, _ := fw.feeds.Get("dshield")
	if !slices.Equal(second.Prefixes, first.Prefixes) || !second.ChangedAt.Equal(first.ChangedAt) ||
		!second.CheckedAt.After(first.CheckedAt) {
		t.Errorf("a 304 changed %+v into %+v", first, second)
	}
	// Review R2#11: a timestamp is not worth a 2.7 MB rewrite. The cache has
	// it; the file keeps the last change.
	if onDisk, _, _ := NewFeedStore(fw.cfg.FeedsPath()).Get("dshield"); !onDisk.CheckedAt.Equal(first.CheckedAt) {
		t.Errorf("a 304 rewrote feeds.json (checked_at %v on disk, %v before)", onDisk.CheckedAt, first.CheckedAt)
	}
}

// Plan P13: a write prunes every copy no rule set names, and keeps one that
// only Backup names — the rules a rollback would restore.
func TestUpdateFeed_PrunesToCurrentStagedAndBackup(t *testing.T) {
	fw := feedFirewall(t, "dshield", "cins", "tor-exits")
	c, _ := captureConn(t)
	fw.nft = &NftablesManager{conn: c}
	for _, id := range []string{"dshield", "cins", "tor-exits"} {
		if _, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: id, Entries: hosts(3)}, "test"); err != nil {
			t.Fatal(err)
		}
	}
	// cins survives in Backup only; tor-exits leaves every rule set.
	if err := fw.rules.SaveStaged("feeds", []string{"dshield", "cins"}); err != nil {
		t.Fatal(err)
	}
	if err := fw.rules.BackupCurrent(); err != nil {
		t.Fatal(err)
	}
	if err := fw.rules.PromoteStaged(); err != nil {
		t.Fatal(err)
	}
	if err := fw.rules.BackupCurrent(); err != nil { // Backup = {dshield, cins}
		t.Fatal(err)
	}
	if err := fw.rules.SaveStaged("feeds", []string{"dshield"}); err != nil {
		t.Fatal(err)
	}
	if err := fw.rules.PromoteStaged(); err != nil { // Current = Staged = {dshield}
		t.Fatal(err)
	}
	if _, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: "dshield", Entries: hosts(4)}, "test"); err != nil {
		t.Fatal(err)
	}
	all, err := NewFeedStore(fw.cfg.FeedsPath()).All()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := all["tor-exits"]; ok {
		t.Error("tor-exits is named by no rule set and was kept")
	}
	if _, ok := all["cins"]; !ok {
		t.Error("cins is in Backup — a rollback restores it — and its copy was pruned")
	}
	if all["dshield"].Entries != 4 {
		t.Errorf("dshield = %+v", all["dshield"])
	}
}

// GET_FEEDS: every id stored or switched on, with Stored true for every copy
// (the web process sends conditional requests only for those), the addr
// question answered from the copy, the staged allowlist's overlap counted, and
// the counters said to be unread when they could not be.
func TestFeeds(t *testing.T) {
	fw := feedFirewall(t, "dshield", "cins")
	// 2001:db9::/32, not 2001:db8::/32 — the latter is IPv6's documentation
	// range and, since the D4-4 revision, dropped rather than stored.
	if _, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: "dshield",
		Entries: []string{"198.51.100.0/24", "2001:db9::/32"}}, "test"); err != nil {
		t.Fatal(err)
	}
	if err := fw.rules.SaveStaged("allowlist", []string{"198.51.100.7", "# the office", "203.0.113.0/24",
		"198.51.0.0/16", "2001:db9:1::/48"}); err != nil {
		t.Fatal(err)
	}

	res, err := fw.Feeds("::ffff:198.51.100.9")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Feeds) != 2 || res.Feeds[0].ID != "cins" || res.Feeds[1].ID != "dshield" {
		t.Fatalf("feeds %+v", res.Feeds)
	}
	cins, dshield := res.Feeds[0], res.Feeds[1]
	if cins.Stored || cins.ContainsAddr || cins.AllowlistOverlap != 0 {
		t.Errorf("cins has no copy: %+v", cins)
	}
	if !dshield.Stored || dshield.Entries != 2 || !dshield.ContainsAddr || dshield.AllowlistOverlap != 3 ||
		dshield.InKernel || dshield.CheckedAt.IsZero() {
		t.Errorf("dshield %+v, want stored, 2 entries, holding the address, 3 allowlist entries covered", dshield)
	}
	if dshield.CountersRead {
		t.Error("with no kernel the counters were reported as read")
	}

	if res, _ := fw.Feeds("203.0.113.1"); res.Feeds[1].ContainsAddr {
		t.Error("an address outside the copy is reported inside it")
	}
	if _, err := fw.Feeds("not an address"); err == nil {
		t.Error("an address that does not parse was answered")
	}
}

func TestOverlapsAny(t *testing.T) {
	rs := mergeRanges(prefixes("198.51.100.0/24", "203.0.113.8/29"))
	for _, tc := range []struct {
		r    addrRange
		want bool
	}{
		{v4Pair("198.51.99.255", "198.51.99.255"), false},
		{v4Pair("198.51.99.255", "198.51.100.0"), true},
		{v4Pair("198.51.100.255", "198.51.100.255"), true},
		{v4Pair("198.51.101.0", "203.0.113.7"), false},
		{v4Pair("198.51.101.0", "203.0.113.8"), true},
		{v4Pair("203.0.113.16", "203.0.113.16"), false},
		{v4Pair("0.0.0.0", "255.255.255.255"), true},
	} {
		if got := overlapsAny(rs, tc.r); got != tc.want {
			t.Errorf("overlapsAny(%v) = %v, want %v", rangesString([]addrRange{tc.r}), got, tc.want)
		}
	}
	if overlapsAny(nil, v4Pair("1.1.1.1", "1.1.1.1")) {
		t.Error("an empty feed overlaps")
	}
}

func v4Pair(from, to string) addrRange {
	return addrRange{netip.MustParseAddr(from), netip.MustParseAddr(to)}
}

// A write that fails leaves the cache as it was: the next apply must load what
// is on disk, not what failed to get there.
func TestFeedStore_AFailedWriteChangesNothing(t *testing.T) {
	s := NewFeedStore(t.TempDir() + "/no/such/dir/feeds.json")
	err := s.Put("dshield", storedFeed{Prefixes: []string{"198.51.100.0/24"}, Entries: 1}, []string{"dshield"})
	if err == nil {
		t.Fatal("a write into a directory that does not exist succeeded")
	}
	if got, _ := s.Contents([]string{"dshield"}); len(got) != 0 {
		t.Errorf("the failed write is in the cache: %v", got)
	}
}

// And errors.Is sees the sentinel through the wrapping, so the daemon's
// errResp carries the exact text.
func TestUpdateFeed_PanicIsTheSentinel(t *testing.T) {
	fw := feedFirewall(t, "dshield")
	if err := EngagePanic(fw.cfg.PanicMarkerPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: "dshield", Entries: hosts(1)}, "test"); !errors.Is(err, ErrPanicEngaged) {
		t.Errorf("UpdateFeed under panic = %v", err)
	}
}

// The two commands reach the handlers through dispatch, and a refusal comes
// back as Response.Error — for the slot, exactly the text the web matches.
func TestDispatch_TheFeedCommands(t *testing.T) {
	fw := feedFirewall(t, "dshield")
	d := &Daemon{cfg: fw.cfg, firewall: fw, quit: make(chan struct{})}

	payload, _ := json.Marshal(shared.UpdateFeedPayload{ID: "dshield", Entries: []string{"198.51.100.0/24"}})
	resp := d.dispatch(shared.Command{Type: shared.CmdUpdateFeed, Payload: payload})
	var res shared.UpdateFeedResult
	if !resp.Success || json.Unmarshal(resp.Data, &res) != nil || res.After != 1 || !res.Changed {
		t.Fatalf("UPDATE_FEED: %+v / %+v", resp, res)
	}

	payload, _ = json.Marshal(shared.GetFeedsPayload{Addr: "198.51.100.1"})
	resp = d.dispatch(shared.Command{Type: shared.CmdGetFeeds, Payload: payload})
	var feeds shared.GetFeedsResult
	if !resp.Success || json.Unmarshal(resp.Data, &feeds) != nil || len(feeds.Feeds) != 1 ||
		!feeds.Feeds[0].Stored || !feeds.Feeds[0].ContainsAddr {
		t.Fatalf("GET_FEEDS: %+v / %+v", resp, feeds)
	}
	if resp := d.dispatch(shared.Command{Type: shared.CmdGetFeeds}); !resp.Success {
		t.Errorf("GET_FEEDS with no payload: %+v", resp)
	}

	fw.beginApply()
	resp = d.dispatch(shared.Command{Type: shared.CmdUpdateFeed, Payload: payload})
	if resp.Success || resp.Error != shared.ErrApplyInProgressText {
		t.Errorf("UPDATE_FEED with the slot held: %+v, want exactly %q", resp, shared.ErrApplyInProgressText)
	}
	fw.endApply()

	for _, c := range []shared.CommandType{shared.CmdUpdateFeed, shared.CmdGetFeeds} {
		if resp := d.dispatch(shared.Command{Type: c, Payload: []byte(`{invalid`)}); resp.Success ||
			!strings.HasPrefix(resp.Error, "invalid payload") {
			t.Errorf("%s with a broken payload: %+v", c, resp)
		}
	}
}

// ReplaceFeedSet writes the kernel outside an apply, so it answers to the rules
// the apply's writers do, read off the source like theirs: it is called from
// UpdateFeed alone, after the panic marker was checked and the apply slot was
// taken (plan P7), with the slot's release deferred. It has no panic check
// after it because it cannot recreate a table
// (TestIntegration_ARefreshCannotRecreateATornDownTable), and no counter
// booking before it because it does not reset one
// (TestIntegration_ARefreshKeepsTheCounter).
func TestTheRefreshWritesOnlyUnderTheSlot(t *testing.T) {
	const write = "f.nft.ReplaceFeedSet("
	sources := coreSources(t)
	for file, src := range sources {
		if n := len(indexesOf(src, write)); n != map[bool]int{true: 1, false: 0}[file == "feeds.go"] {
			t.Errorf("%s calls ReplaceFeedSet %d times; UpdateFeed in feeds.go is its one caller", file, n)
		}
	}
	body := funcBody(t, sources["feeds.go"], "feeds.go", "func (f *Firewall) UpdateFeed(")
	writes := indexesOf(body, write)
	panics := indexesOf(body, "f.PanicEngaged()")
	slots := indexesOf(body, "if !f.beginApply() {")
	releases := indexesOf(body, "defer f.endApply()")
	if len(writes) != 1 || len(slots) != 1 || len(releases) != 1 || len(panics) == 0 {
		t.Fatalf("UpdateFeed: %d writes, %d slot claims, %d deferred releases, %d panic checks; want 1, 1, 1 and at least 1",
			len(writes), len(slots), len(releases), len(panics))
	}
	if panics[0] > slots[0] || slots[0] > releases[0] || releases[0] > writes[0] {
		t.Error("UpdateFeed must check the panic marker, then claim the apply slot and defer its release, " +
			"and only then write the kernel")
	}
}

// Rulings X1: the shrink guard protects the set the kernel is running, and only
// that. A list that really shrank — or an own feed whose URL now names a
// shorter list, which the core cannot tell apart — is refused while live, with
// the exact text the web process matches, and accepted after the operator
// switches it off, applies, and switches it on again; the apply after that
// loads the smaller copy.
func TestUpdateFeed_AShrunkFeedIsAcceptedAfterOffApplyOn(t *testing.T) {
	for _, id := range []string{"dshield", "own-1"} {
		t.Run(id, func(t *testing.T) {
			fw := liveFeedFirewall(t, []string{id}, id)
			if _, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: id, Entries: hosts(20)}, "test"); err != nil {
				t.Fatal(err)
			}
			_, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: id, Entries: hosts(10)}, "test")
			if resp := errResp(err); resp.Error != shared.ErrFeedShrankText {
				t.Fatalf("a live feed shrinking to half: %q, want exactly %q", resp.Error, shared.ErrFeedShrankText)
			}

			if err := fw.rules.SaveStaged("feeds", []string{}); err != nil { // off
				t.Fatal(err)
			}
			promote(t, fw)                                                     // apply
			if err := fw.rules.SaveStaged("feeds", []string{id}); err != nil { // on, staged
				t.Fatal(err)
			}
			res, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: id, Entries: hosts(10)}, "test")
			if err != nil || res.After != 10 || res.Loaded {
				t.Fatalf("the first fetch after switching it back on: %+v, %v", res, err)
			}
			promote(t, fw) // apply
			state, _ := fw.rules.GetState()
			if got := fw.feedContents(state.Current)[id]; len(got) != 10 {
				t.Errorf("the apply loads %d entries, want the accepted 10", len(got))
			}
		})
	}
}

// A 304 skips the rewrite only when nothing else would change: a copy no rule
// set names any more is still pruned from the file.
func TestUpdateFeed_ANotModifiedStillPrunes(t *testing.T) {
	fw := feedFirewall(t, "dshield", "cins")
	for _, id := range []string{"dshield", "cins"} {
		if _, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: id, Entries: hosts(3)}, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if err := fw.rules.SaveStaged("feeds", []string{"dshield"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fw.UpdateFeed(shared.UpdateFeedPayload{ID: "dshield", NotModified: true}, "test"); err != nil {
		t.Fatal(err)
	}
	if all, _ := NewFeedStore(fw.cfg.FeedsPath()).All(); len(all) != 1 {
		t.Errorf("feeds.json holds %d copies after a 304, want cins pruned", len(all))
	}
}
