package core

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// UPDATE_FEED and GET_FEEDS (2.23, spec §3). The web process fetches and
// parses; the core trusts none of it.

// nonGlobalV4 is what spec §3 step 4 names: RFC 1918, 100.64/10, 127/8,
// 169.254/16, 0/8 and 224/3, which includes 240/4. A prefix that overlaps any
// of them is dropped and counted — FireHOL level1 carries 10/8 and 192.168/16,
// and a feed that blocks the operator's own LAN is the lockout spec D4 exists
// for. Not shared.BogonRanges: that list is the bogon filter's and carries the
// documentation ranges, which are no danger in a feed.
var nonGlobalV4 = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("224.0.0.0/3"),
}

// globalV6 is IPv6's global unicast space (RFC 4291 §2.4). Everything outside
// it — ::1, ::, fc00::/7, fe80::/10, ff00::/8 and what is still unassigned —
// is dropped: the v6 equivalents of the list above, and then some.
var globalV6 = netip.MustParsePrefix("2000::/3")

// docV6 is IPv6's documentation range (RFC 3849), carved out of otherwise
// global space so nobody ships it as a reachable address. Unlike IPv4's
// TEST-NETs (RFC 5737) — real, if unrouted, addresses a feed may legitimately
// carry as examples, and which nonGlobalV4 leaves alone — 2001:db8::/32 is
// excluded here (review finding, D4-4 revision).
var docV6 = netip.MustParsePrefix("2001:db8::/32")

// isGlobal reports whether p lies in globally routed space.
func isGlobal(p netip.Prefix) bool {
	if p.Addr().Is6() {
		return globalV6.Bits() <= p.Bits() && globalV6.Contains(p.Addr()) && !docV6.Overlaps(p)
	}
	for _, n := range nonGlobalV4 {
		if n.Overlaps(p) {
			return false
		}
	}
	return true
}

// entirelyNonGlobal reports whether p lies wholly inside a single non-global
// range — narrower than or equal to it — as opposed to merely overlapping
// one. Spec §3 step 4 (the drop) runs on this before step 5 (the breadth
// refusal): a feed that carries its own bogon documentation verbatim —
// FireHOL level1 lists 224.0.0.0/3, 10.0.0.0/8 and 100.64.0.0/10 among its
// entries — has those lines dropped and counted, not the whole update refused
// as "broader than /8" (review finding, D4-4 revision; the earlier order was
// the plan's own §3 reading, corrected by review).
//
// A prefix that only overlaps non-global space in part — broader than the
// range, like 100.0.0.0/8 over 100.64.0.0/10, or unbounded like 0.0.0.0/0 and
// ::/0 — is not "wholly inside" anything here; it still reaches the breadth
// check, and isGlobal's overlap-based drop once that has passed.
func entirelyNonGlobal(p netip.Prefix) bool {
	if p.Addr().Is6() {
		if !globalV6.Overlaps(p) {
			return true
		}
		return docV6.Bits() <= p.Bits() && docV6.Contains(p.Addr())
	}
	for _, n := range nonGlobalV4 {
		if n.Bits() <= p.Bits() && n.Contains(p.Addr()) {
			return true
		}
	}
	return false
}

// feedPrefix parses one entry the web process sent: a bare address or a
// prefix, an IPv4-mapped one unmapped, masked. A zone is not an address a
// packet arrives from. ok is false for anything else; the caller names the
// entry by its position, never by its text, because the refusal reaches the
// audit log and that log takes no text the web process chose.
func feedPrefix(e string) (netip.Prefix, bool) {
	if a, err := netip.ParseAddr(e); err == nil {
		a = a.Unmap()
		return netip.PrefixFrom(a, a.BitLen()), a.Zone() == ""
	}
	p, err := netip.ParsePrefix(e)
	if err != nil {
		return netip.Prefix{}, false
	}
	if p.Addr().Is4In6() {
		if p.Bits() < 96 {
			return netip.Prefix{}, false
		}
		p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
	}
	return p.Masked(), true
}

// validateFeedEntries is spec §3 steps 3 to 5: at most FeedMaxEntries, each
// parsed again, what is not globally routable dropped and counted, then none
// of what remains broader than /8 or /16. It returns the kept prefixes
// deduplicated and sorted, which is the form feeds.json stores and Changed
// compares.
//
// The drop runs before the breadth check, not after (review finding, D4-4
// revision): a prefix wholly inside non-global space — a feed's own bogon
// documentation, like FireHOL level1's 224.0.0.0/3 — is dropped and counted
// there, so the breadth check never sees it and never refuses the whole
// update over it. 0.0.0.0/0 and ::/0 are not wholly inside any single
// non-global range — they overlap all of them, and everything else — so they
// still reach, and fail, the breadth check.
func validateFeedEntries(entries []string) (kept []netip.Prefix, dropped int, err error) {
	if len(entries) > shared.FeedMaxEntries {
		return nil, 0, fmt.Errorf("%d entries; a feed may hold at most %d", len(entries), shared.FeedMaxEntries)
	}
	seen := make(map[netip.Prefix]bool, len(entries))
	for i, e := range entries {
		p, ok := feedPrefix(strings.TrimSpace(e))
		if !ok {
			return nil, 0, fmt.Errorf("entry %d is not an address or a prefix", i+1)
		}
		if entirelyNonGlobal(p) {
			dropped++
			continue
		}
		if (p.Addr().Is4() && p.Bits() < shared.FeedMaxBitsV4) || (p.Addr().Is6() && p.Bits() < shared.FeedMaxBitsV6) {
			return nil, 0, fmt.Errorf("%s is broader than /%d (IPv4) or /%d (IPv6)", p, shared.FeedMaxBitsV4, shared.FeedMaxBitsV6)
		}
		if !isGlobal(p) {
			dropped++
			continue
		}
		if !seen[p] {
			seen[p] = true
			kept = append(kept, p)
		}
	}
	slices.SortFunc(kept, func(a, b netip.Prefix) int {
		if c := a.Addr().Compare(b.Addr()); c != 0 {
			return c
		}
		return a.Bits() - b.Bits()
	})
	return kept, dropped, nil
}

// shrinks is spec D5: a version under FeedShrinkPercent of the stored count is
// refused. A first load has no stored count.
func shrinks(before, after int) bool {
	return after*100 < before*shared.FeedShrinkPercent
}

// feedIDs is Current ∪ Staged ∪ Backup's enabled feeds — the copies feeds.json
// keeps (plan P13).
func feedIDs(state shared.RulesState) []string {
	var ids []string
	for _, r := range []shared.Rules{state.Current, state.Staged, state.Backup} {
		for _, id := range r.Feeds {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// UpdateFeed is UPDATE_FEED: spec §3 steps 1–9, in that order. Every refusal
// leaves the previous version in the kernel and on disk.
//
// The apply slot is held from step 2 to the end (plan P7). An apply cycle
// holds it for its whole acceptance window, and an update arriving then is
// refused with ErrApplyInProgress rather than queued: the set it would write
// is about to be rebuilt from Current, and the file it would write is the one
// the rollback reads. The web process keeps its ETag and tries again later.
func (f *Firewall) UpdateFeed(p shared.UpdateFeedPayload, user string) (shared.UpdateFeedResult, error) {
	// 1. Panic mode: nothing reaches the kernel, and nothing is stored for a
	// resume to find either.
	if f.PanicEngaged() {
		return shared.UpdateFeedResult{}, ErrPanicEngaged
	}
	if !f.beginApply() {
		return shared.UpdateFeedResult{}, ErrApplyInProgress
	}
	defer f.endApply()

	// 2. An id the core knows, switched on in Staged or Current. An unknown id
	// is a web process doing something no version of it does, so it is audited
	// — without the id, which is text that process chose. A known id that is
	// not switched on is a refresh racing the switch that turned it off.
	if !shared.KnownFeedID(p.ID) {
		f.auditFeedRefused("", "an id that is not a feed", user)
		return shared.UpdateFeedResult{}, errors.New("not a feed id")
	}
	state, err := f.rules.GetState()
	if err != nil {
		return shared.UpdateFeedResult{}, fmt.Errorf("get rules: %w", err)
	}
	inCurrent := slices.Contains(state.Current.Feeds, p.ID)
	if !inCurrent && !slices.Contains(state.Staged.Feeds, p.ID) {
		return shared.UpdateFeedResult{}, fmt.Errorf("feed %s is not switched on", p.ID)
	}

	stored, had, err := f.feeds.Get(p.ID)
	if err != nil {
		return shared.UpdateFeedResult{}, err
	}
	now := time.Now().UTC()
	res := shared.UpdateFeedResult{Before: stored.Entries}

	// A 304: checked_at moves and nothing else (plan P8). With no stored copy
	// there is nothing it confirms.
	if p.NotModified {
		if !had {
			return shared.UpdateFeedResult{}, fmt.Errorf("feed %s has no stored copy for a 304 to confirm", p.ID)
		}
		stored.CheckedAt = now
		res.After, res.Dropped = stored.Entries, stored.Dropped
		return res, f.feeds.Put(p.ID, stored, feedIDs(state))
	}

	// 3–5. Parsed again, bounded, stripped.
	kept, dropped, err := validateFeedEntries(p.Entries)
	if err != nil {
		f.auditFeedRefused(p.ID, err.Error(), user)
		return shared.UpdateFeedResult{}, fmt.Errorf("feed %s refused: %w", p.ID, err)
	}
	// Nothing left is a broken list, not an empty one: the web process refuses
	// a 200 with nothing it can parse, and this is the same refusal after the
	// core's own filtering. Stored, it would also leave the shrink guard with
	// nothing to measure the next version against.
	if len(kept) == 0 {
		reason := fmt.Sprintf("no entry is globally routable (%d dropped)", dropped)
		f.auditFeedRefused(p.ID, reason, user)
		return shared.UpdateFeedResult{}, fmt.Errorf("feed %s refused: %s", p.ID, reason)
	}
	// 6. Shrink only (D5), and only for the copy the kernel is running
	// (rulings X1): the guard is against a truncated download replacing a
	// live set. A feed that is staged but not yet applied, or switched off,
	// takes any size — which is how an operator accepts a list that really
	// did shrink, or an own feed whose URL now names a shorter list: switch
	// it off and apply, switch it on, and the next fetch is stored.
	if had && inCurrent && shrinks(stored.Entries, len(kept)) {
		f.auditFeedRefused(p.ID, fmt.Sprintf("%d entries is under %d %% of the %d stored",
			len(kept), shared.FeedShrinkPercent, stored.Entries), user)
		return shared.UpdateFeedResult{}, errors.New(shared.ErrFeedShrankText)
	}

	next := storedFeed{Prefixes: prefixStrings(kept), Entries: len(kept), Dropped: dropped,
		ChangedAt: stored.ChangedAt, CheckedAt: now}
	res.After, res.Dropped = len(kept), dropped
	res.Changed = !had || !slices.Equal(stored.Prefixes, next.Prefixes)
	if res.Changed {
		next.ChangedAt = now
	}

	// 8 before 7 (rulings X2): the kernel first, the file second, so a copy
	// the kernel refuses is never the one the next apply — or its rollback —
	// loads. Live feeds are loaded changed or not, so a load that failed last
	// time is not left behind by a version that has not changed since. The
	// slot is held, so Current cannot change under this. The rule counters are
	// not booked first: a set flush leaves them alone (ReplaceFeedSet), where
	// an apply's rebuild zeroes them.
	if inCurrent {
		v4, v6 := splitFamilies(mergeRanges(kept))
		if err := f.nft.ReplaceFeedSet(p.ID, v4, v6); err != nil {
			if f.PanicEngaged() {
				return shared.UpdateFeedResult{}, ErrPanicEngaged
			}
			f.auditFeedRefused(p.ID, "the kernel refused the new copy: "+err.Error(), user)
			return shared.UpdateFeedResult{}, fmt.Errorf("load feed %s: %w", p.ID, err)
		}
		res.Loaded = true
	}

	// 7. The file, atomically. Failing after a load, the kernel holds the new
	// copy and the file the old one, until the next refresh stores it or the
	// next apply puts the old one back.
	if err := f.feeds.Put(p.ID, next, feedIDs(state)); err != nil {
		slog.Error("a feed's new copy is loaded and could not be stored", "feed", p.ID, "error", err)
		return res, err
	}

	// 9. Only a version that changed is an event. An hourly list that did not
	// change would put 24 lines a day per feed into a log the interface shows
	// 200 lines of, and push the applies out of it.
	if res.Changed {
		detail := fmt.Sprintf("%s: %d → %d entries, %d not globally routable dropped", p.ID, res.Before, res.After, dropped)
		if res.Loaded {
			detail += ", loaded"
		}
		WriteAuditLog(f.cfg.AuditLogPath(), "feed_updated", "feeds", detail, user)
	}
	return res, nil
}

func (f *Firewall) auditFeedRefused(id, reason, user string) {
	detail := reason
	if id != "" {
		detail = id + ": " + reason
	}
	WriteAuditLog(f.cfg.AuditLogPath(), "feed_refused", "feeds", detail, user)
}

// Feeds is GET_FEEDS: counts, timestamps and counters, never entries — one per
// id stored or switched on in Current or Staged. With addr, each says whether
// its copy holds it, which is how the lockout verdict learns of a feed hit.
//
// The counters are read live (plan P16), under the nft mutex. During a slow
// custom-rules apply that read waits, and the web's short deadline can expire
// first; the card then renders without packet counts.
func (f *Firewall) Feeds(addr string) (shared.GetFeedsResult, error) {
	var a netip.Addr
	if addr != "" {
		var err error
		if a, err = netip.ParseAddr(addr); err != nil {
			return shared.GetFeedsResult{}, fmt.Errorf("invalid address: %w", err)
		}
		a = a.Unmap().WithZone("")
	}
	state, err := f.rules.GetState()
	if err != nil {
		return shared.GetFeedsResult{}, fmt.Errorf("get rules: %w", err)
	}
	stored, err := f.feeds.All()
	if err != nil {
		return shared.GetFeedsResult{}, err
	}
	ids := slices.Concat(state.Current.Feeds, state.Staged.Feeds)
	for id := range stored {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)

	counters, cErr := f.nft.RuleCounters()
	out := shared.GetFeedsResult{Feeds: make([]shared.FeedStatus, 0, len(ids))}
	for _, id := range ids {
		rec, ok := stored[id]
		st := shared.FeedStatus{
			ID: id, Stored: ok, Entries: rec.Entries, Dropped: rec.Dropped,
			ChangedAt: rec.ChangedAt, CheckedAt: rec.CheckedAt,
			InKernel:         slices.Contains(state.Current.Feeds, id),
			CountersRead:     cErr == nil,
			AllowlistOverlap: f.feeds.AllowlistOverlap(id, state.Staged.Allowlist),
			ContainsAddr:     f.feeds.Contains(id, a),
		}
		if cErr == nil {
			st.Packets = counters[feedCounterID(id)].Packets
		}
		out.Feeds = append(out.Feeds, st)
	}
	return out, nil
}
