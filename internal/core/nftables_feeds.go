package core

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"github.com/google/nftables/userdata"
	"github.com/jp1337/easywall/internal/shared"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

// Other people's lists in the kernel (2.23): one interval set per enabled feed
// and address family, and one rule each, after the allowlist.

// FeedContents is what Apply loads into the feed sets: enabled id → the
// validated prefixes the core stored for it. An enabled id with no entry gets
// an empty set.
type FeedContents map[string][]netip.Prefix

// feedSetName names the set of one feed and family: "feed-<id>-v4" / "-v6".
func feedSetName(id string, v6 bool) string {
	if v6 {
		return "feed-" + id + "-v6"
	}
	return "feed-" + id + "-v4"
}

// feedCounterID is the reserved rule id on a feed's drop rule. Reserved, so
// usage and Health skip it (IsReservedRuleID); GET_FEEDS reads it.
func feedCounterID(id string) string { return "_feed-" + id }

// addrRange is an inclusive span of addresses of one family.
type addrRange struct{ from, to netip.Addr }

// lastAddr is the highest address in p.
func lastAddr(p netip.Prefix) netip.Addr {
	b := p.Masked().Addr().As16()
	host := 128 - p.Bits()
	if p.Addr().Is4() {
		host = 32 - p.Bits()
	}
	for i := 15; i >= 0 && host > 0; i-- {
		n := min(host, 8)
		b[i] |= byte(1<<n - 1)
		host -= n
	}
	a := netip.AddrFrom16(b)
	if p.Addr().Is4() {
		return a.Unmap()
	}
	return a
}

// mergeRanges turns prefixes into sorted, disjoint, non-adjacent ranges: the
// only shape the kernel's interval set accepts from raw netlink. nft merges in
// userspace; the kernel refuses an element that overlaps another (plan P4 —
// TestIntegration_TheKernelRefusesOverlappingIntervals). IPv4 sorts before
// IPv6 (netip.Addr.Compare), and the two never merge: a v6 start compares
// above every v4 address, and the address after the top of IPv4 is invalid.
func mergeRanges(ps []netip.Prefix) []addrRange {
	rs := make([]addrRange, 0, len(ps))
	for _, p := range ps {
		if p.IsValid() {
			rs = append(rs, addrRange{p.Masked().Addr(), lastAddr(p)})
		}
	}
	slices.SortFunc(rs, func(a, b addrRange) int { return a.from.Compare(b.from) })
	out := rs[:0]
	for _, r := range rs {
		if n := len(out); n > 0 {
			last := &out[n-1]
			next := last.to.Next() // invalid past the top of the family
			if r.from.Compare(last.to) <= 0 || (next.IsValid() && r.from == next) {
				if r.to.Compare(last.to) > 0 {
					last.to = r.to
				}
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

// splitFamilies returns the IPv4 and the IPv6 ranges of a mergeRanges result.
func splitFamilies(rs []addrRange) (v4, v6 []addrRange) {
	i, _ := slices.BinarySearchFunc(rs, 0, func(r addrRange, _ int) int {
		if r.from.Is4() {
			return -1
		}
		return 1
	})
	return rs[:i], rs[i:]
}

// rangeElements is one range as the interval set's element pair: the first
// address, and the address after the last flagged as the interval's end. A
// range that runs to the top of its family has no address after it and is
// sent as its start alone, which the kernel reads as open-ended.
func rangeElements(rs []addrRange) []nftables.SetElement {
	out := make([]nftables.SetElement, 0, 2*len(rs))
	for _, r := range rs {
		out = append(out, nftables.SetElement{Key: r.from.AsSlice()})
		if end := r.to.Next(); end.IsValid() {
			out = append(out, nftables.SetElement{Key: end.AsSlice(), IntervalEnd: true})
		}
	}
	return out
}

// Bytes one range adds to a SetAddElements call's element list, measured from
// what google/nftables v0.3.0 marshals (TestFeedRangeBytesAreWhatTheLibrarySends):
// the start element and the end element, each a nested attribute.
const (
	feedRangeBytesV4 = 40
	feedRangeBytesV6 = 64
)

// maxElemListBytes is the payload one NFTA_SET_ELEM_LIST_ELEMENTS attribute
// can carry. Its length is a uint16, and mdlayher/netlink v1.11.2 truncates a
// longer one silently (attribute.go:92) — the kernel then reads garbage
// (spec §3 step 8, research §code finding D).
const maxElemListBytes = 0xffff - 4

// feedRangesPerChunk is how many ranges of one family fit in one call.
func feedRangesPerChunk(v6 bool) int {
	if v6 {
		return maxElemListBytes / feedRangeBytesV6
	}
	return maxElemListBytes / feedRangeBytesV4
}

// feedElementBytes is what ranges add to the batch, with a margin per call for
// the message and set headers. The send buffer is sized from it (plan P3).
func feedElementBytes(v4, v6 []addrRange) int {
	calls := len(v4)/feedRangesPerChunk(false) + len(v6)/feedRangesPerChunk(true) + 2
	return len(v4)*feedRangeBytesV4 + len(v6)*feedRangeBytesV6 + calls*256
}

// addFeedElements queues ranges into set in calls of at most
// maxElemListBytes each, on the connection's pending batch — the caller's
// Flush sends them with whatever else is queued, in one transaction.
func (m *NftablesManager) addFeedElements(set *nftables.Set, ranges []addrRange) error {
	per := feedRangesPerChunk(set.KeyType.Bytes == 16)
	for len(ranges) > 0 {
		n := min(per, len(ranges))
		if err := m.conn.SetAddElements(set, rangeElements(ranges[:n])); err != nil {
			return fmt.Errorf("feed set %s: %w", set.Name, err)
		}
		ranges = ranges[n:]
	}
	return nil
}

// feedSet is the set one feed and family live in.
func feedSet(t *nftables.Table, id string, f addrFamily) *nftables.Set {
	return &nftables.Set{
		Table:    t,
		Name:     feedSetName(id, f.nfproto == unix.NFPROTO_IPV6),
		KeyType:  f.keyType,
		Interval: true,
	}
}

// addFeeds writes, for every id in enabled and both families, the set, its
// elements and the two rules — the rate-limited log rule, then
// `ip saddr @feed-<id>-v4 counter drop` carrying feedCounterID(id). Apply calls
// it between the allowlist accepts and the port accepts (spec §4, D3). It
// returns what the elements add to the batch, for the send buffer.
func (m *NftablesManager) addFeeds(t *nftables.Table, c *nftables.Chain, enabled []string,
	feeds FeedContents, opts shared.FirewallOptions) (int, error) {
	// The two errors below cannot happen with these arguments — AddSet refuses
	// only an anonymous set, SetAddElements only an anonymous set or a key it
	// cannot marshal. Were one to, the messages queued so far stay in the
	// connection until the next Flush; the next writer's own DelTable is what
	// discards them.
	batch := 0
	for _, id := range enabled {
		v4, v6 := splitFamilies(mergeRanges(feeds[id]))
		batch += feedElementBytes(v4, v6)
		for _, f := range addrFamilies {
			set := feedSet(t, id, f)
			if err := m.conn.AddSet(set, nil); err != nil {
				return 0, fmt.Errorf("feed set %s: %w", set.Name, err)
			}
			ranges := v4
			if f.nfproto == unix.NFPROTO_IPV6 {
				ranges = v6
			}
			if err := m.addFeedElements(set, ranges); err != nil {
				return 0, err
			}
			match := append(srcAddrExprs(f), &expr.Lookup{SourceRegister: 1, SetName: set.Name, SetID: set.ID})
			m.addLogged(t, c, match, logSpec{
				enabled:   opts.LogFeed,
				prefix:    shared.FeedLogPrefix(id),
				perMinute: opts.LogFeedLimit,
			}, userdata.AppendString(nil, userdata.TypeComment, feedCounterID(id)),
				&expr.Counter{}, &expr.Verdict{Kind: expr.VerdictDrop})
		}
	}
	return batch, nil
}

// feedSockOption is the send-buffer half of plan P3, on every socket this
// manager's connection dials. A batch is one sendmsg (mdlayher/netlink
// conn_linux.go:106-119), and the kernel refuses one longer than the socket's
// send buffer with EMSGSIZE (net/netlink/af_netlink.c:1857 @ v6.12) — 212 992
// bytes by default, about 5 000 IPv4 ranges. m.sndbuf is set by the one flush
// that carries feed elements and is zero otherwise.
//
// SO_SNDBUFFORCE needs CAP_NET_ADMIN in the initial user namespace; the core
// has it. Refused, SO_SNDBUF still raises the buffer as far as
// net.core.wmem_max allows, and m.sndbufCapped makes the EMSGSIZE that may
// follow say why.
func (m *NftablesManager) feedSockOption(c *netlink.Conn) error {
	if m.sndbuf == 0 {
		return nil
	}
	// Without NETLINK_CAP_ACK the kernel echoes every refused message in its
	// error ack. Sixty-odd refused 64 KiB element calls overflow the receive
	// buffer, and the error an operator reads is ENOBUFS instead of the
	// kernel's own reason (TestIntegration_ARefusedLargeBatchSaysWhy).
	if err := c.SetOption(netlink.CapAcknowledge, true); err != nil {
		return err
	}
	raw, err := c.SyscallConn()
	if err != nil {
		return err
	}
	var serr error
	if err := raw.Control(func(fd uintptr) {
		serr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_SNDBUFFORCE, m.sndbuf)
	}); err != nil {
		return err
	}
	if errors.Is(serr, unix.EPERM) {
		m.sndbufCapped = true
		return c.SetWriteBuffer(m.sndbuf)
	}
	return serr
}

// flushLarge is Flush for a batch that may carry up to batch bytes of feed
// elements beyond the rules.
func (m *NftablesManager) flushLarge(batch int) error {
	if batch > 0 {
		// The rules themselves fitted the default buffer before 2.23, so that
		// is the margin on top of the elements.
		m.sndbuf, m.sndbufCapped = batch+defaultSndbuf, false
		defer func() { m.sndbuf = 0 }()
	}
	err := m.conn.Flush()
	if err != nil && m.sndbufCapped && errors.Is(err, unix.EMSGSIZE) {
		return fmt.Errorf("%w: the feed sets need a %d-byte send buffer, and without "+
			"CAP_NET_ADMIN the socket cannot be given more than net.core.wmem_max", err, m.sndbuf)
	}
	return err
}

// errNothingWritten marks an ApplyWithFeeds error from a batch the kernel
// refused whole: the previous table, counters included, is still in force.
// The rollback does not reset the usage baselines after one.
var errNothingWritten = errors.New("nothing was written to the kernel")

// defaultSndbuf is net.core.wmem_default on a stock kernel.
const defaultSndbuf = 212992
