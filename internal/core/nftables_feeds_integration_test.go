//go:build integration

package core

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/nftables"
	"github.com/jp1337/easywall/internal/shared"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

// rawIntervalSet builds table inet easywall with one interval set holding
// exactly these ranges, unmerged, in one flush.
func rawIntervalSet(t *testing.T, m *NftablesManager, rs []addrRange) error {
	t.Helper()
	tbl := m.conn.AddTable(easywallInetTable())
	set := feedSet(tbl, "probe", addrFamilies[0])
	if err := m.conn.AddSet(set, nil); err != nil {
		t.Fatal(err)
	}
	if err := m.conn.SetAddElements(set, rangeElements(rs)); err != nil {
		t.Fatal(err)
	}
	return m.conn.Flush()
}

func v4Range(from, to string) addrRange {
	return addrRange{netip.MustParseAddr(from), netip.MustParseAddr(to)}
}

// nftGetElement asks the kernel whether addr is in set.
func nftGetElement(t *testing.T, set, addr string) bool {
	t.Helper()
	out, err := exec.Command("nft", "get", "element", "inet", tableName, set, "{", addr, "}").CombinedOutput()
	t.Logf("nft get element %s { %s }: %v %s", set, addr, err, strings.TrimSpace(string(out)))
	return err == nil
}

// setHolds asks the kernel whether set holds a — NFT_MSG_GETSETELEM for one
// key, the lookup `nft get element` makes, without the CLI listing the whole
// set first: on 100 000 intervals that listing runs for minutes, and a
// GetSetElements dump takes tens of seconds. One socket per question, so an
// error cannot leave a stale reply for the next one to read. ENOENT is "not
// in the set"; any other error is asked again.
func setHolds(t *testing.T, set string, a netip.Addr) bool {
	t.Helper()
	key, _ := netlink.MarshalAttributes([]netlink.Attribute{{Type: unix.NFTA_DATA_VALUE, Data: a.AsSlice()}})
	elem, _ := netlink.MarshalAttributes([]netlink.Attribute{{Type: unix.NFTA_SET_ELEM_KEY | unix.NLA_F_NESTED, Data: key}})
	list, _ := netlink.MarshalAttributes([]netlink.Attribute{{Type: 1 | unix.NLA_F_NESTED, Data: elem}})
	attrs, _ := netlink.MarshalAttributes([]netlink.Attribute{
		{Type: unix.NFTA_SET_ELEM_LIST_TABLE, Data: []byte(tableName + "\x00")},
		{Type: unix.NFTA_SET_ELEM_LIST_SET, Data: []byte(set + "\x00")},
		{Type: unix.NFTA_SET_ELEM_LIST_ELEMENTS | unix.NLA_F_NESTED, Data: list},
	})
	msg := netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType(unix.NFNL_SUBSYS_NFTABLES<<8 | unix.NFT_MSG_GETSETELEM),
			Flags: netlink.Request | netlink.Acknowledge,
		},
		Data: append([]byte{unix.NFPROTO_INET, 0, 0, 0}, attrs...),
	}
	for try := 0; try < 100; try++ {
		c, err := netlink.Dial(unix.NETLINK_NETFILTER, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Execute(msg)
		_ = c.Close()
		switch {
		case err == nil:
			return true
		case errors.Is(err, unix.ENOENT):
			return false
		}
	}
	t.Fatalf("GETSETELEM %s %s never got an answer", set, a)
	return false
}

// Plan P4, measured: an interval set written over raw netlink refuses a range
// that overlaps or nests in another with EEXIST (the kernel turns the rbtree's
// ENOTEMPTY into EEXIST, nf_tables_api.c:7192-7196 @ v6.12). nft merges in
// userspace before it sends anything, so `nft add element` proves nothing
// either way. Adjacent ranges are accepted; mergeRanges joins them anyway,
// which halves the elements a list of neighbouring /24s costs.
//
// Then the same prefixes through ApplyWithFeeds, which merges: the set loads,
// and holds both ends.
func TestIntegration_TheKernelRefusesOverlappingIntervals(t *testing.T) {
	for _, tc := range []struct {
		name string
		rs   []addrRange
	}{
		{"overlapping", []addrRange{v4Range("198.18.0.0", "198.18.0.255"), v4Range("198.18.0.128", "198.18.1.127")}},
		{"nested", []addrRange{v4Range("198.18.0.0", "198.18.0.255"), v4Range("198.18.0.2", "198.18.0.2")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newIntegrationManager(t)
			err := rawIntervalSet(t, m, tc.rs)
			if !errors.Is(err, unix.EEXIST) {
				t.Fatalf("an unmerged %s pair gave %v, want EEXIST — if the kernel now merges, "+
					"plan P4's reason for mergeRanges has changed", tc.name, err)
			}
		})
	}

	m := newIntegrationManager(t)
	adj := rawIntervalSet(t, m, []addrRange{v4Range("198.18.0.0", "198.18.0.127"), v4Range("198.18.0.128", "198.18.0.255")})
	t.Logf("adjacent, unmerged: %v", adj)

	rules := shared.Rules{Feeds: []string{"dshield"}}
	feeds := FeedContents{"dshield": {
		netip.MustParsePrefix("198.18.0.0/24"),
		netip.MustParsePrefix("198.18.0.128/25"),
		netip.MustParsePrefix("198.18.0.2/32"),
		netip.MustParsePrefix("198.18.1.0/24"),
	}}
	if err := m.ApplyWithFeeds(shared.RulesState{Current: rules}, shared.FirewallOptions{},
		shared.NetworkSettings{}, feeds); err != nil {
		t.Fatalf("ApplyWithFeeds with overlapping prefixes: %v", err)
	}
	for _, a := range []string{"198.18.0.2", "198.18.1.255"} {
		if !nftGetElement(t, "feed-dshield-v4", a) {
			t.Errorf("%s is not in the merged set", a)
		}
	}
	if nftGetElement(t, "feed-dshield-v4", "198.18.2.0") {
		t.Error("198.18.2.0 is in the set; the merge overshot")
	}
}

// A range that runs to the last address of its family has no end element
// (rangeElements), and the kernel reads a start with no end as open-ended.
func TestIntegration_ARangeToTheTopOfTheFamilyIsOpenEnded(t *testing.T) {
	m := newIntegrationManager(t)
	feeds := FeedContents{"dshield": {netip.MustParsePrefix("255.255.255.0/24"), netip.MustParsePrefix("ffff::/16")}}
	if err := m.ApplyWithFeeds(shared.RulesState{Current: shared.Rules{Feeds: []string{"dshield"}}},
		shared.FirewallOptions{}, shared.NetworkSettings{}, feeds); err != nil {
		t.Fatal(err)
	}
	for set, in := range map[string][]string{
		"feed-dshield-v4": {"255.255.255.0", "255.255.255.255"},
		"feed-dshield-v6": {"ffff::", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"},
	} {
		for _, a := range in {
			if !nftGetElement(t, set, a) {
				t.Errorf("%s is not in %s", a, set)
			}
		}
	}
	if nftGetElement(t, "feed-dshield-v4", "255.255.254.255") {
		t.Error("the address before the range is in the set")
	}
}

// listenOn accepts and closes connections on 10.77.1.1:port until the test ends.
func listenOn(t *testing.T, port int) {
	t.Helper()
	ln, err := net.Listen("tcp", "10.77.1.1:"+strconv.Itoa(port))
	if err != nil {
		t.Skipf("skipping: cannot listen on 10.77.1.1:%d: %v", port, err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
}

const feedTestPort = 12241

// Spec §4 and D3: the allowlist accepts before any feed drops, and the feeds
// drop before any port accepts. Pinned by the kernel's own rendering of the
// input chain, and then by a packet: a source that is in a feed and on the
// allowlist reaches the port; in the feed alone, it does not; in neither, it
// does (the control that makes the second answer mean something).
func TestIntegration_AllowlistIsEvaluatedBeforeFeeds(t *testing.T) {
	for _, bin := range []string{"bash", "timeout"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("skipping: %s is not installed", bin)
		}
	}
	m := newIntegrationManager(t)
	r := newRouter(t)
	listenOn(t, feedTestPort)

	open := []shared.PortRule{{Port: strconv.Itoa(feedTestPort), Description: "feed test", ID: "fee0d0000001"}}
	feeds := FeedContents{"dshield": {netip.MustParsePrefix("10.77.1.0/24")}}
	opts := shared.FirewallOptions{LogFeed: true, LogFeedLimit: 30}

	apply := func(rules shared.Rules) {
		t.Helper()
		if err := m.ApplyWithFeeds(shared.RulesState{Current: rules}, opts, shared.NetworkSettings{}, feeds); err != nil {
			t.Fatalf("ApplyWithFeeds: %v", err)
		}
	}

	apply(shared.Rules{TCP: open})
	if !tcpReaches(t, r.pidA, "10.77.1.1", feedTestPort) {
		t.Fatal("control: with no feed enabled the port is not reachable; the harness is not routing")
	}

	apply(shared.Rules{TCP: open, Allowlist: []string{"10.77.1.2"}, Feeds: []string{"dshield"}})
	input := inputChainText(t, m)
	allow := indexOfRule(input, "ip saddr 10.77.1.2", "accept")
	logRule := indexOfRule(input, "ip saddr @feed-dshield-v4", `log prefix "easywall feed: dshield "`)
	drop := indexOfRule(input, "ip saddr @feed-dshield-v4", "counter", "drop", `comment "_feed-dshield"`)
	drop6 := indexOfRule(input, "ip6 saddr @feed-dshield-v6", "counter", "drop", `comment "_feed-dshield"`)
	port := indexOfRule(input, "dport "+strconv.Itoa(feedTestPort), "accept")
	if allow < 0 || logRule < 0 || drop < 0 || drop6 < 0 || port < 0 {
		t.Fatalf("a rule is missing (allow %d, log %d, drop %d, drop6 %d, port %d):\n  %s",
			allow, logRule, drop, drop6, port, strings.Join(input, "\n  "))
	}
	if allow >= logRule || logRule != drop-1 || drop >= port || drop6 >= port {
		t.Errorf("want allowlist accept < feed log = drop-1 < port accept; got allow %d, log %d, "+
			"drop %d, drop6 %d, port %d:\n  %s", allow, logRule, drop, drop6, port, strings.Join(input, "\n  "))
	}
	if !tcpReaches(t, r.pidA, "10.77.1.1", feedTestPort) {
		t.Error("an allowlisted source in a feed was dropped: the feed outranks the allowlist")
	}

	apply(shared.Rules{TCP: open, Feeds: []string{"dshield"}})
	if tcpReaches(t, r.pidA, "10.77.1.1", feedTestPort) {
		t.Errorf("a source in an enabled feed reached an open port:\n  %s",
			strings.Join(inputChainText(t, m), "\n  "))
	}
	counters, err := m.RuleCounters()
	if err != nil {
		t.Fatal(err)
	}
	if counters[feedCounterID("dshield")].Packets == 0 {
		t.Errorf("the feed's drop counted nothing under %s: %v", feedCounterID("dshield"), counters)
	}
}

// Spec §3: a feed with no stored copy yet gets an empty set, and its rule.
// And panic mode's teardown takes the sets with the table.
func TestIntegration_AFeedWithNoCopyHasAnEmptySet(t *testing.T) {
	m := newIntegrationManager(t)
	if err := m.ApplyWithFeeds(shared.RulesState{Current: shared.Rules{Feeds: []string{"cins", "own-2"}}},
		shared.FirewallOptions{}, shared.NetworkSettings{}, nil); err != nil {
		t.Fatal(err)
	}
	for _, set := range []string{"feed-cins-v4", "feed-cins-v6", "feed-own-2-v4", "feed-own-2-v6"} {
		out, err := exec.Command("nft", "list", "set", "inet", tableName, set).CombinedOutput()
		if err != nil {
			t.Fatalf("nft list set %s: %v\n%s", set, err, out)
		}
		if strings.Contains(string(out), "elements") {
			t.Errorf("%s has elements with no copy stored:\n%s", set, out)
		}
	}
	if indexOfRule(inputChainText(t, m), "@feed-own-2-v6", "drop") < 0 {
		t.Error("no drop rule for an enabled feed without a copy")
	}

	if err := m.Reset(); err != nil {
		t.Fatal(err)
	}
	out, _ := exec.Command("nft", "list", "sets", "inet").CombinedOutput()
	if strings.Contains(string(out), "feed-") {
		t.Errorf("a feed set outlived the teardown:\n%s", out)
	}
}

// syntheticFeed is n distinct, non-adjacent global addresses — the worst case
// for elements, since nothing merges.
func syntheticFeed(n int, v6 bool, seed uint32) []netip.Prefix {
	out := make([]netip.Prefix, 0, n)
	for i := 0; i < n; i++ {
		v := seed + uint32(i)*2
		if v6 {
			a := netip.AddrFrom16([16]byte{0x2a, 0x0e, byte(seed >> 8), byte(seed), 0, 0, 0, 0, 0, 0, 0, 0,
				byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
			out = append(out, netip.PrefixFrom(a, 128))
			continue
		}
		a := netip.AddrFrom4([4]byte{byte(64 + v>>24), byte(v >> 16), byte(v >> 8), byte(v)})
		out = append(out, netip.PrefixFrom(a, 32))
	}
	return out
}

// The catalogue at the sizes spec §1 measured on 2026-09-24. Spamhaus's 91
// IPv6 networks are the only IPv6 counted; the other lists' family split was
// not measured.
var catalogueSizes = map[string][2]int{
	"spamhaus-drop": {1710, 91}, "dshield": {20, 0}, "blocklist-de": {32002, 0},
	"cins": {15000, 0}, "et-compromised": {686, 0}, "ipsum-3": {19549, 0},
	"hagezi-tif": {79175, 0}, "tor-exits": {1370, 0},
}

// Spec §3: the rebuild was not atomic — reset() committed a table with no
// chains, which filters nothing, and the rules followed in a second flush.
// Measured before the fix with this harness: about 1 ms with no feed, 325 ms
// with one feed of 100 000 addresses, 476 ms with the catalogue. ApplyWithFeeds
// now deletes and recreates the table in the transaction that fills it, so
// there is no such moment: an observer on its own socket, listing the chains
// as fast as it can while the applies run, must never find the input chain
// missing.
//
// The large cases need a send buffer only CAP_NET_ADMIN in the initial user
// namespace can grant (plan P3); on a host whose net.core.wmem_max is the
// stock 212 992 they fail under `unshare -rn`. Run them with `sudo unshare -n`.
func TestIntegration_ApplyNeverLeavesTheTableWithoutChains(t *testing.T) {
	m := newIntegrationManager(t)
	base := shared.Rules{TCP: []shared.PortRule{{Port: "22", Description: "ssh", ID: "fee0d0000022"}}}
	if err := m.Apply(shared.RulesState{Current: base}, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatal(err)
	}
	obs, err := nftables.New()
	if err != nil {
		t.Fatal(err)
	}

	one := FeedContents{"hagezi-tif": syntheticFeed(shared.FeedMaxEntries, false, 1)}
	all := FeedContents{}
	var ids []string
	seed := uint32(1)
	for id, n := range catalogueSizes {
		ids = append(ids, id)
		all[id] = append(syntheticFeed(n[0], false, seed<<20), syntheticFeed(n[1], true, seed)...)
		seed++
	}

	for _, tc := range []struct {
		name  string
		ids   []string
		feeds FeedContents
	}{
		{"no feed", nil, nil},
		{"one feed, 100 000 IPv4", []string{"hagezi-tif"}, one},
		{"the catalogue", ids, all},
	} {
		rules := base
		rules.Feeds = tc.ids

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
				chains, err := obs.ListChains()
				looks++
				found := false
				for _, c := range chains {
					if err == nil && c.Table.Name == tableName && c.Name == inputChainName {
						found = true
					}
				}
				if !found {
					missing++
				}
			}
		}()

		var slowest time.Duration
		for i := 0; i < 5; i++ {
			start := time.Now()
			if err := m.ApplyWithFeeds(shared.RulesState{Current: rules}, shared.FirewallOptions{},
				shared.NetworkSettings{}, tc.feeds); err != nil {
				close(stop)
				<-result
				// A host whose 2·wmem_max is under the batch cannot run this
				// without CAP_NET_ADMIN in the initial namespace. CI runs as
				// root and sets requireSelftestEnv, where this fails.
				if m.sndbufCapped && errors.Is(err, unix.EMSGSIZE) {
					skipOrFailUnprovable(t, fmt.Sprintf("%s: the send buffer cannot grow without root: %v", tc.name, err))
				}
				t.Fatalf("%s: %v", tc.name, err)
			}
			slowest = max(slowest, time.Since(start))
		}
		close(stop)
		got := <-result
		fmt.Printf("APPLY %-24s slowest %10s  observer looked %6d times, input chain missing %d\n",
			tc.name, slowest, got[0], got[1])
		if got[1] > 0 {
			t.Errorf("%s: the observer found table inet easywall without its input chain %d times "+
				"in %d looks — the rebuild is not one transaction", tc.name, got[1], got[0])
		}
	}
}

// Spec §3: rollback, boot and resume need nothing of their own, because they
// rebuild the table through the writers that load the stored copies. One
// firewall, all three writers: an accepted apply switches a feed on, an
// unconfirmed apply switching it off rolls back to it, and a boot restore of a
// deleted table brings it back — each time with the stored copy in the set.
func TestIntegration_EveryWriterLoadsTheStoredFeeds(t *testing.T) {
	fw := newTestFirewallWithRealNft(t)
	fw.feeds = NewFeedStore(fw.cfg.FeedsPath())
	writeFeedsFile(t, fw.cfg.DataDir, `{"dshield": {"prefixes": ["198.51.100.0/24"], "entries": 1}}`)
	fw.cfg.Acceptance.Duration = 1
	fw.acceptance = NewAcceptance(fw.cfg.AcceptanceDuration())

	loaded := func(writer string) {
		t.Helper()
		if !nftGetElement(t, "feed-dshield-v4", "198.51.100.7") {
			t.Errorf("after %s the feed's set does not hold its stored copy", writer)
		}
	}

	_ = fw.rules.SaveStaged("tcp", []shared.PortRule{{Port: "22", Description: "ssh"}})
	if err := fw.rules.SaveStaged("feeds", []string{"dshield"}); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		fw.Accept()
	}()
	if err := fw.Apply("integration-test"); err != nil {
		t.Fatal(err)
	}
	loaded("an accepted apply")

	if err := fw.rules.SaveStaged("feeds", []string{}); err != nil {
		t.Fatal(err)
	}
	if err := fw.Apply("integration-test"); err != nil { // not confirmed: rolls back after 1 s
		t.Fatal(err)
	}
	loaded("the rollback")

	fw.nft.conn.DelTable(easywallInetTable())
	if err := fw.nft.conn.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := fw.RestoreCurrent(RestoreReasonBoot); err != nil {
		t.Fatal(err)
	}
	loaded("the boot restore")
}

// Plan P3: one batch is one sendmsg, refused with EMSGSIZE past the socket's
// send buffer. Three full feeds are about 12 MB. As root the buffer is forced
// to fit and the apply succeeds; without CAP_NET_ADMIN in the initial user
// namespace SO_SNDBUFFORCE is refused, SO_SNDBUF stops at net.core.wmem_max,
// and the error has to say that rather than "message too long". Whichever of
// the two this host is, the test asserts that one.
func TestIntegration_TheSendBufferGrowsOrTheErrorSaysWhy(t *testing.T) {
	m := newIntegrationManager(t)
	feeds := FeedContents{}
	ids := []string{"own-1", "own-2", "own-3"}
	for i, id := range ids {
		feeds[id] = syntheticFeed(shared.FeedMaxEntries, false, uint32(i+1)<<21)
	}
	err := m.ApplyWithFeeds(shared.RulesState{Current: shared.Rules{Feeds: ids}},
		shared.FirewallOptions{}, shared.NetworkSettings{}, feeds)
	t.Logf("capped=%v: %v", m.sndbufCapped, err)
	switch {
	case err == nil:
		if !setHolds(t, "feed-own-3-v4", syntheticFeed(1, false, 3<<21)[0].Addr()) {
			t.Error("the apply succeeded and the last feed's first address is not in its set")
		}
	case !m.sndbufCapped:
		t.Errorf("the buffer was forced and the apply still failed: %v", err)
	case !errors.Is(err, unix.EMSGSIZE) || !strings.Contains(err.Error(), "net.core.wmem_max"):
		t.Errorf("a capped buffer failed with %v; want EMSGSIZE naming net.core.wmem_max", err)
	}
}

// Plan P18: the rebuild is one transaction, so a batch the kernel refuses
// leaves the previous table in force — rules and counters — and says so with
// errNothingWritten. A bogus delete queued ahead of the apply makes the kernel
// refuse the whole batch.
func TestIntegration_ARefusedBatchLeavesThePreviousTable(t *testing.T) {
	m := newIntegrationManager(t)
	rules := func(port string) shared.RulesState {
		return shared.RulesState{Current: shared.Rules{TCP: []shared.PortRule{{Port: port, Description: "p", ID: "fee0d000" + port}}}}
	}
	if err := m.Apply(rules("2222"), shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatal(err)
	}
	m.conn.DelTable(&nftables.Table{Name: "r2-nope", Family: nftables.TableFamilyINet})
	err := m.Apply(rules("3333"), shared.FirewallOptions{}, shared.NetworkSettings{})
	if !errors.Is(err, errNothingWritten) {
		t.Fatalf("a refused batch returned %v, want errNothingWritten", err)
	}
	input := strings.Join(inputChainText(t, m), "\n")
	if !strings.Contains(input, "2222") || strings.Contains(input, "3333") {
		t.Errorf("after a refused batch the input chain holds:\n%s\nwant the previous rules (2222) and not the refused ones", input)
	}
	if err := m.Apply(rules("3333"), shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Errorf("the next apply failed: %v", err)
	}
}

// Rulings X3: a batch whose 100 000 elements the kernel refuses reports the
// kernel's reason. Every refused 64 KiB element call is echoed in its error ack
// unless NETLINK_CAP_ACK is set, and sixty-odd echoes overflow the receive
// buffer: the error read "no buffer space available". Here the set is missing
// — the table was torn down, as panic mode does — so every call is refused.
func TestIntegration_ARefusedLargeBatchSaysWhy(t *testing.T) {
	m := newIntegrationManager(t)
	if err := m.Reset(); err != nil {
		t.Fatal(err)
	}
	v4, _ := splitFamilies(mergeRanges(syntheticFeed(shared.FeedMaxEntries, false, 1)))
	set := feedSet(easywallInetTable(), "hagezi-tif", addrFamilies[0])
	if err := m.addFeedElements(set, v4); err != nil {
		t.Fatal(err)
	}
	err := m.flushLarge(feedElementBytes(v4, nil))
	if err == nil {
		t.Fatal("elements for a set that does not exist were accepted")
	}
	if m.sndbufCapped && errors.Is(err, unix.EMSGSIZE) {
		skipOrFailUnprovable(t, fmt.Sprintf("the send buffer cannot grow without root: %v", err))
	}
	if errors.Is(err, unix.ENOBUFS) || !errors.Is(err, unix.ENOENT) {
		t.Errorf("the refusal reads %v; want the kernel's ENOENT, not ENOBUFS", err)
	}
}

// Review R2#5: a rollback whose batch the kernel refused whole leaves the
// previous table and its counters, so the usage baselines must stay as they
// are. The baseline here belongs to a staged rule with no kernel counter, so
// the collect before the write leaves it at 100 and only a reset could move it.
func TestIntegration_ARefusedRollbackKeepsTheBaselines(t *testing.T) {
	fw := newTestFirewallWithRealNft(t)
	fw.feeds = NewFeedStore(fw.cfg.FeedsPath())
	fw.setLastApply(time.Now()) // everConfigured: the rollback applies, it does not tear down
	if err := fw.rules.SaveStaged("tcp", []shared.PortRule{{Port: "22", Description: "ssh"}}); err != nil {
		t.Fatal(err)
	}
	state, _ := fw.rules.GetState()
	id := state.Staged.TCP[0].ID
	if err := fw.usage.write(shared.UsageResult{Usage: map[string]shared.RuleUsage{id: {Packets: 5, KernelPackets: 100}}}); err != nil {
		t.Fatal(err)
	}
	fw.nft.conn.DelTable(&nftables.Table{Name: "r2-nope", Family: nftables.TableFamilyINet})
	fw.rollback(state, "test")
	res, err := fw.usage.Read()
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Usage[id].KernelPackets; got != 100 {
		t.Errorf("the baseline is %d after a refused rollback, want 100 untouched", got)
	}
}

// portRules is n TCP port rules from port first, each with an IPv4 and an IPv6
// source — the shape of root01xvp's rule set, where 2.23.0 failed.
func portRules(n, first int) shared.Rules {
	r := shared.Rules{}
	for i := 0; i < n; i++ {
		r.TCP = append(r.TCP, shared.PortRule{ID: fmt.Sprintf("%012x", first+i), Port: strconv.Itoa(first + i),
			Sources: []string{"198.51.100.0/24", "2001:db8::/32"}})
	}
	return r
}

// 2.23.1: the kernel answers every rule — an ack and an echo, queued after the
// commit — and on 2.23.0 those answers overflowed the stock 212 992-byte
// receive buffer at about 104 kernel rules with the curated feeds and about
// 150 without (2.22.0 had the second limit already). root01xvp: ~107 rules and
// four feeds, nine apply_failed. Both shapes must apply. Without CAP_NET_ADMIN
// in the initial user namespace the buffers stop at net.core.rmem_max and
// wmem_max; a host whose limits are below the need skips, and CI — root, with
// requireSelftestEnv — fails instead.
func TestIntegration_ARealisticRuleSetWithFeedsApplies(t *testing.T) {
	feeds := FeedContents{
		"blocklist-de":   syntheticFeed(25237, false, 1<<20),
		"spamhaus-drop":  syntheticFeed(1800, false, 2<<20),
		"et-compromised": syntheticFeed(679, false, 3<<20),
		"dshield":        syntheticFeed(20, false, 4<<20),
	}
	withFeeds := portRules(40, 1000)
	withFeeds.Feeds = []string{"spamhaus-drop", "dshield", "blocklist-de", "et-compromised"}
	for _, tc := range []struct {
		name  string
		rules shared.Rules
		feeds FeedContents
	}{
		{"40 port rules and the four root01xvp feeds", withFeeds, feeds},
		{"160 port rules, no feed", portRules(160, 1000), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newIntegrationManager(t)
			err := m.ApplyWithFeeds(shared.RulesState{Current: tc.rules}, allProtectionModulesOn(),
				shared.NetworkSettings{}, tc.feeds)
			t.Logf("%s: %d kernel rules, sndbuf capped=%v, rcvbuf capped=%v: %v",
				tc.name, len(m.built), m.sndbufCapped, m.rcvbufCapped, err)
			if err != nil {
				// Only a cap below what the manager asked for excuses a
				// failure: an estimate too small fails here on any host.
				batch := 0
				for _, ps := range tc.feeds {
					batch += feedElementBytes(splitFamilies(mergeRanges(ps)))
				}
				snd, rcv := flushBuffers(len(m.built), batch)
				if (m.rcvbufCapped && sysctlInt(t, "rmem_max") < rcv) || (m.sndbufCapped && sysctlInt(t, "wmem_max") < snd) {
					skipOrFailUnprovable(t, fmt.Sprintf("%s: the socket buffers cannot grow to %d/%d without root: %v",
						tc.name, snd, rcv, err))
				}
				t.Fatalf("%s: %v", tc.name, err)
			}
			if tc.feeds != nil && !setHolds(t, "feed-blocklist-de-v4", feeds["blocklist-de"][0].Addr()) {
				t.Errorf("%s: the apply succeeded and blocklist-de's first address is not in its set", tc.name)
			}
		})
	}
}

// 2.23.1: the acks and echoes come after the commit, so a flush that fails
// receiving them has written the table. 2.23.0 tagged it errNothingWritten,
// the rollback kept usage baselines describing a table that was gone, and the
// audit log said nothing had been written. The receive buffer is forced small
// here; 40 port rules also overflow the stock one.
func TestIntegration_AReceiveOverflowIsNotNothingWritten(t *testing.T) {
	m := newIntegrationManager(t)
	if err := m.Apply(shared.RulesState{Current: portRules(1, 2000)}, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatal(err)
	}
	m.rcvbufForTest = 4096
	err := m.Apply(shared.RulesState{Current: portRules(40, 3000)}, allProtectionModulesOn(), shared.NetworkSettings{})
	if !errors.Is(err, unix.ENOBUFS) {
		t.Fatalf("a 4096-byte receive buffer returned %v, want ENOBUFS", err)
	}
	if errors.Is(err, errNothingWritten) {
		t.Errorf("a receive overflow after the commit is tagged errNothingWritten: %v", err)
	}
	input := strings.Join(inputChainText(t, m), "\n")
	if strings.Contains(input, "2000") || !strings.Contains(input, "3039") {
		t.Errorf("after the overflow the input chain holds:\n%s\nwant the new rules (3000-3039), not the previous (2000)", input)
	}
}

// sysctlInt reads net.core.<name>.
func sysctlInt(t *testing.T, name string) int {
	t.Helper()
	b, err := os.ReadFile("/proc/sys/net/core/" + name)
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	return n
}
