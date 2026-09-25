package core

import (
	"encoding/binary"
	"net/netip"
	"slices"
	"testing"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"github.com/google/nftables/userdata"
	"github.com/jp1337/easywall/internal/shared"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

func prefixes(ss ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

func rangesString(rs []addrRange) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.from.String() + "-" + r.to.String()
	}
	return out
}

func TestLastAddr(t *testing.T) {
	for in, want := range map[string]string{
		"198.51.100.0/24": "198.51.100.255",
		"198.51.100.7/32": "198.51.100.7",
		"10.0.0.0/8":      "10.255.255.255",
		"0.0.0.0/0":       "255.255.255.255",
		"203.0.113.9/29":  "203.0.113.15", // not masked on the way in
		"2001:db8::/32":   "2001:db8:ffff:ffff:ffff:ffff:ffff:ffff",
		"2001:db8::1/128": "2001:db8::1",
		"2001:db8::/61":   "2001:db8:0:7:ffff:ffff:ffff:ffff",
	} {
		if got := lastAddr(netip.MustParsePrefix(in)).String(); got != want {
			t.Errorf("lastAddr(%s) = %s, want %s", in, got, want)
		}
	}
}

// Plan P4: the kernel refuses an overlapping or nested interval
// (TestIntegration_TheKernelRefusesOverlappingIntervals), so every set is
// written from ranges that are sorted, disjoint and — cheaper, not required —
// not adjacent.
func TestMergeRanges(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []netip.Prefix
		want []string
	}{
		{"empty", nil, []string{}},
		{"disjoint, out of order",
			prefixes("203.0.113.0/24", "198.51.100.0/24"),
			[]string{"198.51.100.0-198.51.100.255", "203.0.113.0-203.0.113.255"}},
		{"overlap", prefixes("198.51.100.0/25", "198.51.100.64/26", "198.51.100.96/27"),
			[]string{"198.51.100.0-198.51.100.127"}},
		{"nested address", prefixes("198.51.100.0/24", "198.51.100.7/32"),
			[]string{"198.51.100.0-198.51.100.255"}},
		{"nested, larger second", prefixes("198.51.100.7/32", "198.51.100.0/24"),
			[]string{"198.51.100.0-198.51.100.255"}},
		{"adjacent", prefixes("198.51.100.0/25", "198.51.100.128/25", "198.51.101.0/32"),
			[]string{"198.51.100.0-198.51.101.0"}},
		{"one address apart is not adjacent", prefixes("198.51.100.1/32", "198.51.100.3/32"),
			[]string{"198.51.100.1-198.51.100.1", "198.51.100.3-198.51.100.3"}},
		{"duplicate", prefixes("198.51.100.1/32", "198.51.100.1/32"),
			[]string{"198.51.100.1-198.51.100.1"}},
		{"a later range inside an earlier merged one",
			prefixes("198.51.100.0/24", "198.51.100.128/25", "198.51.100.200/32"),
			[]string{"198.51.100.0-198.51.100.255"}},
		{"the families never merge: the top of IPv4 and the bottom of IPv6",
			prefixes("::/128", "255.255.255.255/32"),
			[]string{"255.255.255.255-255.255.255.255", "::-::"}},
		{"IPv6", prefixes("2001:db8::/33", "2001:db8:8000::/33", "2001:db8::5/128"),
			[]string{"2001:db8::-2001:db8:ffff:ffff:ffff:ffff:ffff:ffff"}},
		{"unmasked input", prefixes("198.51.100.9/24"), []string{"198.51.100.0-198.51.100.255"}},
		{"zero prefix skipped", []netip.Prefix{{}, netip.MustParsePrefix("198.51.100.1/32")},
			[]string{"198.51.100.1-198.51.100.1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := rangesString(mergeRanges(tc.in)); !slices.Equal(got, tc.want) {
				t.Errorf("mergeRanges = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSplitFamilies(t *testing.T) {
	v4, v6 := splitFamilies(mergeRanges(prefixes("2001:db8::/32", "198.51.100.0/24", "203.0.113.1/32")))
	if len(v4) != 2 || len(v6) != 1 || !v4[1].from.Is4() || !v6[0].from.Is6() {
		t.Errorf("v4 %v, v6 %v", rangesString(v4), rangesString(v6))
	}
	if v4, v6 := splitFamilies(nil); len(v4)+len(v6) != 0 {
		t.Error("nil split into something")
	}
}

// One range is a start element and the address after its last as an interval
// end. At the top of the family there is no such address; the kernel takes a
// start alone as open-ended (TestIntegration_ARangeToTheTopOfTheFamilyIsOpenEnded).
func TestRangeElements(t *testing.T) {
	els := rangeElements(mergeRanges(prefixes("198.51.100.0/24", "255.255.255.0/24")))
	want := []nftables.SetElement{
		{Key: []byte{198, 51, 100, 0}},
		{Key: []byte{198, 51, 101, 0}, IntervalEnd: true},
		{Key: []byte{255, 255, 255, 0}},
	}
	if len(els) != len(want) {
		t.Fatalf("got %d elements %+v, want %d", len(els), els, len(want))
	}
	for i := range want {
		if !slices.Equal(els[i].Key, want[i].Key) || els[i].IntervalEnd != want[i].IntervalEnd {
			t.Errorf("element %d = %+v, want %+v", i, els[i], want[i])
		}
	}
	if k := rangeElements(mergeRanges(prefixes("2001:db8::/32")))[0].Key; len(k) != 16 {
		t.Errorf("an IPv6 key is %d bytes", len(k))
	}
}

// captureConn is a connection whose Flush hands the batch to got instead of a
// kernel.
func captureConn(t *testing.T) (*nftables.Conn, *[]netlink.Message) {
	t.Helper()
	var got []netlink.Message
	c, err := nftables.New(nftables.WithTestDial(func(req []netlink.Message) ([]netlink.Message, error) {
		got = append(got, req...)
		return req, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	return c, &got
}

const newSetElem = netlink.HeaderType(unix.NFNL_SUBSYS_NFTABLES<<8 | unix.NFT_MSG_NEWSETELEM)

// elemListLens returns, for every NEWSETELEM message in msgs, the length the
// NFTA_SET_ELEM_LIST_ELEMENTS attribute's header claims and the length of the
// data that follows it. The two differ when the uint16 wrapped.
func elemListLens(t *testing.T, msgs []netlink.Message) (claimed, actual []int) {
	t.Helper()
	for _, m := range msgs {
		if m.Header.Type != newSetElem {
			continue
		}
		b := m.Data[4:] // nfgenmsg
		for len(b) >= 4 {
			l := int(binary.NativeEndian.Uint16(b[0:2]))
			typ := binary.NativeEndian.Uint16(b[2:4]) &^ unix.NLA_F_NESTED
			if typ == unix.NFTA_SET_ELEM_LIST_ELEMENTS {
				claimed = append(claimed, l-4)
				actual = append(actual, len(b)-4) // the last attribute makeElemList writes
				break
			}
			step := (l + 3) &^ 3
			if step < 4 || step > len(b) {
				t.Fatalf("malformed attribute of length %d", l)
			}
			b = b[step:]
		}
	}
	return claimed, actual
}

func elemBytes(t *testing.T, v6 bool, n int) int {
	t.Helper()
	c, got := captureConn(t)
	set := feedSet(easywallInetTableForTest(), "probe", addrFamilies[map[bool]int{false: 0, true: 1}[v6]])
	rs := mergeRanges(syntheticPrefixes(n, v6))
	if err := c.SetAddElements(set, rangeElements(rs)); err != nil {
		t.Fatal(err)
	}
	if err := c.Flush(); err != nil {
		t.Fatal(err)
	}
	_, actual := elemListLens(t, *got)
	if len(actual) != 1 {
		t.Fatalf("%d NEWSETELEM messages for one call", len(actual))
	}
	return actual[0]
}

// syntheticPrefixes is n distinct, non-adjacent addresses, so nothing merges.
func syntheticPrefixes(n int, v6 bool) []netip.Prefix {
	out := make([]netip.Prefix, n)
	for i := range out {
		v := uint32(i) * 2
		if v6 {
			out[i] = netip.PrefixFrom(netip.AddrFrom16([16]byte{0x20, 0x01, 0x0d, 0xb8, 12: byte(v >> 24),
				13: byte(v >> 16), 14: byte(v >> 8), 15: byte(v)}), 128)
			continue
		}
		out[i] = netip.PrefixFrom(netip.AddrFrom4([4]byte{100, byte(v >> 16), byte(v >> 8), byte(v)}), 32)
	}
	return out
}

// Plan P5, measured rather than computed: what one range adds to a
// SetAddElements call's element list, as google/nftables v0.3.0 marshals it.
// The chunk size is derived from these, so a library that marshals an element
// differently is caught here and not by a kernel reading a wrapped length.
func TestFeedRangeBytesAreWhatTheLibrarySends(t *testing.T) {
	for _, tc := range []struct {
		v6   bool
		want int
	}{{false, feedRangeBytesV4}, {true, feedRangeBytesV6}} {
		one, two := elemBytes(t, tc.v6, 1), elemBytes(t, tc.v6, 2)
		if got := two - one; got != tc.want {
			t.Errorf("v6=%v: one range adds %d bytes, the constant says %d", tc.v6, got, tc.want)
		}
		if one != tc.want {
			t.Errorf("v6=%v: the first range costs %d bytes, the constant says %d", tc.v6, one, tc.want)
		}
	}
}

// Spec §3 step 8: SetAddElements wraps its uint16 length silently above 64 KiB.
// One chunk at the size feedRangesPerChunk allows is the most that fits, and
// every call addFeedElements makes has a header that tells the truth.
func TestAddFeedElementsStaysUnderTheAttributeLimit(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		per := feedRangesPerChunk(v6)
		if got := elemBytes(t, v6, per); got > maxElemListBytes {
			t.Errorf("v6=%v: a chunk of %d ranges is %d bytes, over %d", v6, per, got, maxElemListBytes)
		}
		if got := elemBytes(t, v6, per+1); got <= maxElemListBytes {
			t.Errorf("v6=%v: %d ranges still fit (%d bytes); the chunk is smaller than it need be", v6, per+1, got)
		}

		c, got := captureConn(t)
		m := &NftablesManager{conn: c}
		fam := addrFamilies[0]
		if v6 {
			fam = addrFamilies[1]
		}
		n := 3*per + 7
		if err := m.addFeedElements(feedSet(easywallInetTableForTest(), "probe", fam),
			mergeRanges(syntheticPrefixes(n, v6))); err != nil {
			t.Fatal(err)
		}
		if err := c.Flush(); err != nil {
			t.Fatal(err)
		}
		claimed, actual := elemListLens(t, *got)
		if len(actual) != 4 {
			t.Errorf("v6=%v: %d ranges went out in %d calls, want 4", v6, n, len(actual))
		}
		elements := 0
		for i := range actual {
			if claimed[i] != actual[i] {
				t.Errorf("v6=%v call %d: the header claims %d bytes and %d follow — the length wrapped",
					v6, i, claimed[i], actual[i])
			}
			elements += actual[i]
		}
		if want := n * map[bool]int{false: feedRangeBytesV4, true: feedRangeBytesV6}[v6]; elements != want {
			t.Errorf("v6=%v: %d bytes of elements sent, want %d — a range was lost or doubled", v6, elements, want)
		}
	}
}

// Spec §4: per feed and family, the rate-limited log rule and then
// `saddr @feed-<id>-v<N> counter drop` carrying the reserved id — in the order
// the feeds were switched on. Read from what the recorder saw, so CheckRules
// reads the same rules.
func TestAddFeedsBuildsALogAndACountedDropPerFamily(t *testing.T) {
	for _, logOn := range []bool{true, false} {
		c, _ := captureConn(t)
		rec := &recordingConn{}
		m := &NftablesManager{conn: c, adder: rec, logSink: logSink{nflog: true, group: 7}}
		tbl := easywallInetTableForTest()
		opts := shared.FirewallOptions{LogFeed: logOn, LogFeedLimit: 17}
		if _, err := m.addFeeds(tbl, inputChainForTest(tbl), []string{"dshield", "own-2"},
			FeedContents{"dshield": prefixes("198.51.100.0/24")}, opts); err != nil {
			t.Fatal(err)
		}

		type want struct {
			set string
			log bool
		}
		var wants []want
		for _, id := range []string{"dshield", "own-2"} {
			for _, v6 := range []bool{false, true} {
				if logOn {
					wants = append(wants, want{feedSetName(id, v6), true})
				}
				wants = append(wants, want{feedSetName(id, v6), false})
			}
		}
		if len(rec.rules) != len(wants) {
			t.Fatalf("log=%v: %d rules, want %d", logOn, len(rec.rules), len(wants))
		}
		for i, w := range wants {
			r := rec.rules[i]
			var lookup *expr.Lookup
			for _, e := range r.Exprs {
				if l, ok := e.(*expr.Lookup); ok {
					lookup = l
				}
			}
			if lookup == nil || lookup.SetName != w.set {
				t.Errorf("log=%v rule %d looks up %+v, want %s", logOn, i, lookup, w.set)
				continue
			}
			n := len(r.Exprs)
			if w.log {
				l, ok := r.Exprs[n-1].(*expr.Log)
				if !ok || string(l.Data) != shared.FeedLogPrefix(w.set[5:len(w.set)-3]) || l.Group != 7 {
					t.Errorf("rule %d should log %q to the manager's group: %#v", i, w.set, r.Exprs[n-1])
				}
				if lim, ok := r.Exprs[n-2].(*expr.Limit); !ok || lim.Rate != 17 {
					t.Errorf("rule %d: want a limit of 17 a minute before the log, got %#v", i, r.Exprs[n-2])
				}
				if r.UserData != nil {
					t.Errorf("rule %d: the log rule carries an id; only the drop may", i)
				}
				continue
			}
			_, counter := r.Exprs[n-2].(*expr.Counter)
			v, drop := r.Exprs[n-1].(*expr.Verdict)
			if !counter || !drop || v.Kind != expr.VerdictDrop {
				t.Errorf("rule %d ends %#v %#v, want counter then drop", i, r.Exprs[n-2], r.Exprs[n-1])
			}
			id, _ := userdata.GetString(r.UserData, userdata.TypeComment)
			if want := feedCounterID(w.set[5 : len(w.set)-3]); id != want {
				t.Errorf("rule %d carries id %q, want %q", i, id, want)
			}
		}
		if f := CheckRules(rec.rules, nil); len(f) != 0 {
			t.Errorf("the check flags the feed rules: %v", f)
		}
	}
}

// The reserved prefix keeps a feed's counter out of usage.json and out of the
// health check's port sum, and both already key on it.
func TestAFeedCounterIDIsReserved(t *testing.T) {
	if id := feedCounterID("spamhaus-drop"); !IsReservedRuleID(id) || id != "_feed-spamhaus-drop" {
		t.Errorf("feedCounterID = %q", id)
	}
	if feedSetName("own-3", true) != "feed-own-3-v6" || feedSetName("dshield", false) != "feed-dshield-v4" {
		t.Error("set names")
	}
}
