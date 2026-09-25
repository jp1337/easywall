package shared

import (
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"time"
)

// A refused packet, as easywall-core decoded it from NFLOG.
//
// Typed on purpose. The kernel log is text, and anything reading text is a
// parser in the root process — which 2.24's row rejects in as many words. NFLOG
// hands the core a binary header of fixed structure and the first bytes of the
// packet, so every field below is read out of a known offset rather than out of
// a line somebody could shape.

// PacketLogEntry is one refused packet.
type PacketLogEntry struct {
	// Seq is assigned by the core, strictly increasing, and survives a restart
	// through the spill file. It is the row's identity in the page — the live
	// tail preserves an open drill-down by it.
	Seq  uint64    `json:"seq"`
	Time time.Time `json:"time"`

	// Rule is one of PacketLogRules, or PacketLogRuleOther.
	Rule string `json:"rule"`

	// Feed is the feed id a "feed" row's prefix named — a catalogue id or
	// own-N — and empty for every other rule, for a prefix naming an id no
	// catalogue knows, and for every line a core before 2.23 wrote. Since 2.23.
	Feed string `json:"feed,omitempty"`

	// Hook is "input" or "forward", or empty when NFLOG did not say. Only an
	// input packet can be let in by opening a port.
	Hook  string `json:"hook,omitempty"`
	InDev string `json:"in,omitempty"`

	Family  uint8      `json:"family"` // 4 or 6
	Src     netip.Addr `json:"src"`
	Dst     netip.Addr `json:"dst"`
	Proto   string     `json:"proto"` // tcp, udp, icmp, icmpv6, or the protocol number
	SrcPort uint16     `json:"sport,omitempty"`
	// DstPort is zero when the packet carries no transport header this code
	// read: ICMP, a later fragment, a protocol without ports.
	DstPort  uint16 `json:"dport,omitempty"`
	TCPFlags string `json:"flags,omitempty"` // "SYN", "SYN,ACK", …
	TTL      uint8  `json:"ttl"`             // hop limit for IPv6
	Length   uint16 `json:"len"`
	CtState  string `json:"ct,omitempty"` // new, established, related, untracked
	// Mark is the skb's nfmark (NFULA_MARK), or zero when the kernel set none —
	// which most packets carry as-is, so zero is not itself informative.
	Mark uint32 `json:"mark,omitempty"`
	// ICMP is the type and code of an ICMP or ICMPv6 packet, the first two
	// bytes of its header. Nil for every other protocol, for a later fragment,
	// and for every line a core before 2.22 wrote into the spill file — those
	// replay without it, and nil is the honest reading of "not recorded". A
	// pointer, because type 0 code 0 is an echo reply and must not read as
	// absent.
	ICMP *PacketICMP `json:"icmp,omitempty"`
}

// PacketICMP is an ICMP header's first two bytes. Since 2.22.
type PacketICMP struct {
	Type uint8 `json:"type"`
	Code uint8 `json:"code"`
}

// Remedy says which of /blocked's three row actions are worth offering for a
// packet: not "could this have let it through" — a blocklist entry never lets
// a packet through, it blocks the source's next one — but "could this change
// this packet's verdict, or block its source." The page offers only those: an
// allowlist button on a port-scan row would promise something the chain
// cannot do, and "open 3389" there invites the scanner.
type Remedy struct {
	Allowlist, Blocklist, Open bool
}

// Remedies reads the answer off the input chain's order — protection modules,
// then the blocklist, then the allowlist, then the feeds, then the port rules,
// then the final drop (internal/core/nftables.go Apply; Reachable steps 5–9). If that order
// changes, TestRemediesFollowTheChainOrder is the table to redo.
func (e PacketLogEntry) Remedies() Remedy {
	var r Remedy
	// The allowlist, the blocklist and the port rules all live in the input
	// chain; a forwarded packet is decided by forwarded rules, which this page
	// does not write. No easywall log rule sits in the forward chain today.
	if e.Hook == "forward" {
		return r
	}
	switch e.Rule {
	case "drop", PacketLogRuleOther:
		// The allowlist accepts before the final drop and before every custom
		// rule (the nft CLI appends those after everything netlink wrote).
		// Opening the port would rescue a custom-rule drop too; it is not
		// offered, because it would override a rule the operator wrote.
		r.Allowlist = true
	case "bogon":
		// addBogonFilter is given the allowlist as its exemption list.
		r.Allowlist = true
	case "feed":
		// The feeds are evaluated after the allowlist (spec D3): allowlisting
		// the source is exactly what rescues it from somebody else's list.
		r.Allowlist = true
	}
	r.Blocklist = e.Rule != "blocklist"
	r.Open = e.Rule == "drop" && e.DstPort != 0 &&
		(e.Proto == "tcp" || e.Proto == "udp")
	return r
}

// DropReason says why the final drop refused a packet. A default-drop row means
// no option refused it — nothing accepted it — so its reason is what was
// missing, read off the rules the kernel holds now. A closed enum with a locale
// key per value (blocked_why_<code>), for the reason ReachReason is one: a
// sentence assembled in Go cannot be translated.
type DropReason string

const (
	DropIPv6Blocked     DropReason = "ipv6_blocked"      // IPv6 is set to block now; it drops unlogged
	DropIPv6Passthrough DropReason = "ipv6_passthrough"  // IPv6 is passed through now
	DropICMPAcceptedNow DropReason = "icmp_accepted_now" // {Proto, Type}: the type is accepted now
	DropBlocklistedNow  DropReason = "blocklisted_now"   // the source is on the blocklist now
	DropAllowlistedNow  DropReason = "allowlisted_now"   // the source is on the allowlist now
	DropIPv4Ping        DropReason = "ipv4_ping"         // an IPv4 echo request
	DropICMPType        DropReason = "icmp_type"         // {Proto, Type}: not an accepted type
	DropICMPUntyped     DropReason = "icmp_untyped"      // {Proto}: logged before 2.22 recorded the type
	DropNoPort          DropReason = "no_port"           // no port a rule could match
	DropPortClosed      DropReason = "port_closed"       // {Port, Proto}
	DropPortForwarded   DropReason = "port_forwarded"    // {Port, Proto}: the rule is scope = "forwarded"
	DropPortSources     DropReason = "port_sources"      // {Port, Proto, Sources}
	DropPortOpenNow     DropReason = "port_open_now"     // {Port, Proto}: a rule accepts it now
)

// AllDropReasons is the complete list; the interface's guard pins every one to
// both strict locales.
var AllDropReasons = []DropReason{
	DropIPv6Blocked, DropIPv6Passthrough, DropICMPAcceptedNow, DropBlocklistedNow,
	DropAllowlistedNow, DropIPv4Ping, DropICMPType, DropICMPUntyped, DropNoPort,
	DropPortClosed, DropPortForwarded, DropPortSources, DropPortOpenNow,
}

// DropWhy is a reason and the values its sentence names. Code is empty when
// the row has no reason to give: not a default drop, or a forwarded packet.
type DropWhy struct {
	Code   DropReason
	Params map[string]any
}

// ICMPv4Accepted and ICMPv6Accepted are the types addICMPRules accepts
// (internal/core/nftables.go). Written twice, once here and once as kernel
// rules; core's TestICMPAcceptsAreTheListsDropReasonReads builds the rules and
// compares them with these, so the two cannot drift.
var ICMPv4Accepted = []uint8{0, 3, 11, 12}

// ICMPv6Accepted depends on the two discovery settings. Consulted only in
// filter mode — the other two decide IPv6 before any ICMP rule.
func ICMPv6Accepted(v6 IPv6Config) []uint8 {
	t := []uint8{1, 2, 3, 4, 128, 129}
	if v6.ICMPAllowRouterAdvertisement {
		t = append(t, 133, 134)
	}
	if v6.ICMPAllowNeighborAdvertisement {
		t = append(t, 135, 136)
	}
	return t
}

// ParsedRules is Rules with the blocklist, the allowlist and every port
// rule's Sources already decoded into netip.Prefix. InAnyEntry reparses its
// []string argument on every call; DropReason used to call it up to three
// times per row, and /blocked/rows asks it of every on-screen row, every
// five-second poll. At the realistic upper bound — /import accepts 512 KiB
// (handler_export.go:33), about 10,000 short entries — reparsing that list
// for 200 on-screen rows measured 197ms per poll, and 1.02s at 50,000
// entries (packetlog_dropreason_test.go's TestParseRulesOnceHoldsA10000EntryList);
// parsed once per request, both are unmeasurable. Use ParseRules to build
// one; DropReasonParsed reads it.
type ParsedRules struct {
	Blocklist []netip.Prefix
	Allowlist []netip.Prefix
	TCP       []ParsedPortRule
	UDP       []ParsedPortRule
}

// ParsedPortRule is a PortRule with Sources already decoded. The original
// strings stay embedded — DropReasonParsed still needs them, verbatim, for
// the sentence a port_sources row shows.
type ParsedPortRule struct {
	PortRule
	ParsedSources []netip.Prefix
}

// ParseRules decodes every address list in r once. A comment or a blank
// line is dropped here rather than carried into ParsedSources, the same way
// the rule builders and InAnyEntry skip them, so a later membership test
// needs no second pass to find that out.
func ParseRules(r Rules) ParsedRules {
	tcp := make([]ParsedPortRule, len(r.TCP))
	for i, pr := range r.TCP {
		tcp[i] = ParsedPortRule{PortRule: pr, ParsedSources: parseEntryList(pr.Sources)}
	}
	udp := make([]ParsedPortRule, len(r.UDP))
	for i, pr := range r.UDP {
		udp[i] = ParsedPortRule{PortRule: pr, ParsedSources: parseEntryList(pr.Sources)}
	}
	return ParsedRules{
		Blocklist: parseEntryList(r.Blocklist),
		Allowlist: parseEntryList(r.Allowlist),
		TCP:       tcp,
		UDP:       udp,
	}
}

// parseEntryList is InAnyEntry's own parsing step, run once instead of once
// per call. A bare address becomes a single-host prefix, so a later Contains
// alone decides both shapes — exactly InAnyEntry's two branches, without its
// per-call cost. Networks go through ParseNetwork, the parser every rule
// builder and every other containment check uses, not netip.ParsePrefix: the
// two disagree on a spelling like 10.0.0.0/08, and a stricter double here
// would read a source list open that the rule builders read as covering it.
func parseEntryList(entries []string) []netip.Prefix {
	var out []netip.Prefix
	for _, entry := range entries {
		if IsListComment(entry) {
			continue
		}
		e := strings.TrimSpace(entry)
		if addr, err := netip.ParseAddr(e); err == nil {
			addr = addr.Unmap()
			out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
			continue
		}
		if pfx, err := ParseNetwork(e); err == nil {
			out = append(out, pfx)
		}
	}
	return out
}

// inAnyParsedEntry is InAnyEntry against a list parseEntryList already
// decoded: both a bare address and a network are prefixes by then, so one
// Contains test covers what InAnyEntry needed two branches for.
func inAnyParsedEntry(src netip.Addr, entries []netip.Prefix) bool {
	for _, pfx := range entries {
		if pfx.Contains(src) {
			return true
		}
	}
	return false
}

// DropReason is DropReasonParsed for a caller holding unparsed Rules — every
// test in this package, and no production caller since ParseRules exists:
// parsing the blocklist, the allowlist and every port rule's Sources afresh
// for each of a few hundred on-screen rows, every poll, is the cost
// ParsedRules exists to avoid.
func (e PacketLogEntry) DropReason(r Rules, n NetworkSettings) DropWhy {
	return e.DropReasonParsed(ParseRules(r), n)
}

// DropReasonParsed walks the input chain in nft.Apply's order — the fragment
// drop (prerouting, IPv4 fragments only), loopback, the IPv6 mode, the ping
// and reset meters, established, the ICMP accepts, the other modules, the
// Docker networks, the blocklist, the allowlist, the port rules, the custom
// rules, the final log — and names the first step that would decide this
// packet differently now, or the step that was missing. pr is the rule set
// the kernel holds (Current), already parsed once by ParseRules; n the
// network settings it was applied with.
//
// Steps it does not ask about: the fragment drop, loopback, the meters and
// established cannot reach the final drop; a module that refuses logs under
// its own name; the custom rules are not parsed here (see Reachable); and of
// the Docker networks, only CustomNetworks is consulted — the auto-detected
// bridges cannot be listed from the web process, so a packet from one still
// reads as whatever port or list reason would otherwise apply.
func (e PacketLogEntry) DropReasonParsed(pr ParsedRules, n NetworkSettings) DropWhy {
	// The final log is in the input chain only; no easywall rule logs in the
	// forward chain, so a forwarded "drop" row has no step here to name.
	if e.Rule != "drop" || e.Hook == "forward" {
		return DropWhy{}
	}
	// Unmapped and unzoned for the list lookups, as Reachable does — except
	// for a Family 6 packet, which stays un-unmapped: the kernel's IPv4 list
	// entries match NFPROTO_IPV4 only, and an IPv4-mapped source in an IPv6
	// packet is still an IPv6 packet. Contains refuses an IPv4-mapped IPv6
	// address against an IPv4 prefix, so leaving it mapped is what keeps this
	// packet from reading as covered by an IPv4 list, source or port-source
	// entry the kernel would never have matched it against.
	src := e.Src.WithZone("")
	if e.Family != 6 {
		src = src.Unmap()
	}
	icmp := e.Proto == "icmp" || e.Proto == "icmpv6"
	label := "ICMP"
	if e.Proto == "icmpv6" {
		label = "ICMPv6"
	}

	// The IPv6 mode sits right after loopback. Neither verdict logs, so a
	// default-drop IPv6 row under either mode arrived while the mode was filter.
	// The packet's family, not the address's: the kernel rule matches
	// NFPROTO_IPV6, and an IPv4-mapped source is still an IPv6 packet.
	if e.Family == 6 {
		switch n.IPv6.Mode {
		case IPv6Block:
			return DropWhy{Code: DropIPv6Blocked}
		case IPv6Passthrough:
			return DropWhy{Code: DropIPv6Passthrough}
		}
	}

	// The ICMP accepts, before every module.
	if icmp && e.ICMP != nil {
		accepted := ICMPv4Accepted
		if e.Proto == "icmpv6" {
			accepted = ICMPv6Accepted(n.IPv6)
		}
		if slices.Contains(accepted, e.ICMP.Type) {
			return DropWhy{Code: DropICMPAcceptedNow, Params: map[string]any{"Proto": label, "Type": e.ICMP.Type}}
		}
	}

	// The Docker networks accept before the blocklist (nftables.go's Apply:
	// the CIDR accepts render right after the optional modules, before the
	// blocklist). Only CustomNetworks — the ones the operator named — can be
	// listed here; an auto-detected bridge is the gap the comment above names.
	if n.Docker.Enabled {
		for _, cidr := range n.Docker.CustomNetworks {
			if pfx, err := ParseNetwork(cidr); err == nil && pfx.Contains(src) {
				return DropWhy{}
			}
		}
	}

	// The blocklist drops before the allowlist accepts; either one, now,
	// decides this packet before any port rule.
	if inAnyParsedEntry(src, pr.Blocklist) {
		return DropWhy{Code: DropBlocklistedNow}
	}
	if inAnyParsedEntry(src, pr.Allowlist) {
		return DropWhy{Code: DropAllowlistedNow}
	}

	switch {
	case icmp && e.ICMP == nil:
		return DropWhy{Code: DropICMPUntyped, Params: map[string]any{"Proto": label}}
	case icmp && e.Proto == "icmp" && e.ICMP.Type == 8:
		// Not in ICMPv4Accepted, and ICMP flood only rate-limits: it jumps
		// when a source is over its rate and accepts nothing under it.
		return DropWhy{Code: DropIPv4Ping}
	case icmp:
		return DropWhy{Code: DropICMPType, Params: map[string]any{"Proto": label, "Type": e.ICMP.Type}}
	case (e.Proto != "tcp" && e.Proto != "udp") || e.DstPort == 0:
		return DropWhy{Code: DropNoPort}
	}

	rules := pr.TCP
	if e.Proto == "udp" {
		rules = pr.UDP
	}
	p := map[string]any{"Port": e.DstPort, "Proto": e.Proto}
	var others []string
	forwarded := false
	for _, rule := range rules {
		if !PortInRule(rule.Port, e.DstPort) {
			continue
		}
		if !rule.FiltersHost() {
			forwarded = true
			continue
		}
		switch {
		case len(rule.Sources) == 0:
			return DropWhy{Code: DropPortOpenNow, Params: p}
		case len(rule.ParsedSources) == 0:
			// Only comments: portAcceptRules builds no rule rather than
			// opening the port to everyone.
		case inAnyParsedEntry(src, rule.ParsedSources):
			return DropWhy{Code: DropPortOpenNow, Params: p}
		default:
			for _, s := range rule.Sources {
				if !IsListComment(s) {
					others = append(others, strings.TrimSpace(s))
				}
			}
		}
	}
	switch {
	case len(others) > 0:
		p["Sources"] = strings.Join(others, ", ")
		return DropWhy{Code: DropPortSources, Params: p}
	case forwarded:
		return DropWhy{Code: DropPortForwarded, Params: p}
	}
	return DropWhy{Code: DropPortClosed, Params: p}
}

// PacketLogRules are the eleven log switches, by the name their prefix
// carries. filters.md lists them in this order.
var PacketLogRules = []string{
	"ssh", "icmp_flood", "syn_flood", "tcp_rst", "portscan",
	"invalid", "fragment", "bogon", "blocklist", "feed", "drop",
}

// PacketLogRuleOther is a packet some other rule logged into easywall's group —
// a custom rule, usually. Shown, and labelled as not one of the eleven.
const PacketLogRuleOther = "other"

// PacketLogProtos are the protocols the filter offers by name.
var PacketLogProtos = []string{"tcp", "udp", "icmp", "icmpv6"}

// logPrefixStem is how every easywall log prefix begins; the core's constants
// in nftables.go are "easywall <name>: ".
const logPrefixStem = "easywall "

// feedLogPrefixStem is the one prefix that carries a value: the feed's id.
const feedLogPrefixStem = logPrefixStem + "feed: "

// FeedLogPrefix is the log prefix of feed id's drop rule (plan P10). The
// kernel log reads "easywall feed: spamhaus-drop IN=…". The kernel refuses a
// prefix longer than NF_LOG_PREFIXLEN - 1 = 127 bytes, and
// TestFeedLogPrefixesFitTheKernel holds every id to that.
func FeedLogPrefix(id string) string { return feedLogPrefixStem + id + " " }

// ParseLogPrefix names the rule behind an NFLOG prefix and, for a feed's rule,
// the feed. An id that is not a known feed id decodes as rule "feed" with no
// id: the packet was refused by a feed, and which one is not a thing to guess.
func ParseLogPrefix(prefix string) (rule, feed string) {
	name, ok := strings.CutPrefix(prefix, logPrefixStem)
	if !ok {
		return PacketLogRuleOther, ""
	}
	name = strings.TrimRight(name, "\x00 ")
	if id, ok := strings.CutPrefix(name, "feed:"); ok {
		if id = strings.TrimSpace(id); KnownFeedID(id) {
			return "feed", id
		}
		return "feed", ""
	}
	name = strings.ReplaceAll(strings.TrimSuffix(name, ":"), "-", "_")
	// A rule 2.22 loaded keeps its prefix, "easywall blacklist: ", until the
	// next apply.
	name = CurrentListName(name)
	if slices.Contains(PacketLogRules, name) {
		return name, ""
	}
	return PacketLogRuleOther, ""
}

// RuleFromPrefix names the rule behind an NFLOG prefix — ParseLogPrefix's
// first half.
func RuleFromPrefix(prefix string) string {
	rule, _ := ParseLogPrefix(prefix)
	return rule
}

const (
	packetLogDefaultLimit = 200 // what the page shows; the audit log shows as many
	packetLogMaxLimit     = 1000
)

// PacketLogFilter narrows GET_PACKET_LOG. Every field is optional and they
// combine with AND.
type PacketLogFilter struct {
	Src   string `json:"src,omitempty"`  // an address or a CIDR
	Dst   string `json:"dst,omitempty"`  // an address or a CIDR
	Port  uint16 `json:"port,omitempty"` // the destination port — a source port is ephemeral noise
	Proto string `json:"proto,omitempty"`
	Rule  string `json:"rule,omitempty"`
	InDev string `json:"in,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

// ifaceName is what Linux allows in an interface name, and no more: IFNAMSIZ
// is sixteen including the NUL.
var ifaceName = regexp.MustCompile(`^[A-Za-z0-9._@-]{1,15}$`)

// Validate refuses a filter this code cannot interpret. Called by the web
// process at its trust boundary and again by the core.
func (f PacketLogFilter) Validate() error {
	for _, v := range []struct{ name, val string }{{"src", f.Src}, {"dst", f.Dst}} {
		if v.val == "" {
			continue
		}
		if _, err := addrOrPrefix(v.val); err != nil {
			return fmt.Errorf("%s: %w", v.name, err)
		}
	}
	if f.Proto != "" && !slices.Contains(PacketLogProtos, f.Proto) {
		return fmt.Errorf("proto: %q is not one of %v", f.Proto, PacketLogProtos)
	}
	if f.Rule != "" && f.Rule != PacketLogRuleOther && !slices.Contains(PacketLogRules, f.Rule) {
		return fmt.Errorf("rule: %q is not a rule easywall logs", f.Rule)
	}
	if f.InDev != "" && !ifaceName.MatchString(f.InDev) {
		return fmt.Errorf("in: %q is not an interface name", f.InDev)
	}
	if f.Limit < 0 {
		return fmt.Errorf("limit: %d is negative", f.Limit)
	}
	return nil
}

// IsZero reports whether no field narrows anything.
func (f PacketLogFilter) IsZero() bool { return f == PacketLogFilter{} }

// EffectiveLimit is how many entries a query returns at most.
func (f PacketLogFilter) EffectiveLimit() int {
	if f.Limit <= 0 {
		return packetLogDefaultLimit
	}
	return min(f.Limit, packetLogMaxLimit)
}

// Matcher compiles the filter once, so a query over twenty thousand entries
// parses the addresses once rather than twenty thousand times.
func (f PacketLogFilter) Matcher() (func(PacketLogEntry) bool, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	var src, dst netip.Prefix
	if f.Src != "" {
		src, _ = addrOrPrefix(f.Src)
	}
	if f.Dst != "" {
		dst, _ = addrOrPrefix(f.Dst)
	}
	return func(e PacketLogEntry) bool {
		return (!src.IsValid() || src.Contains(e.Src.Unmap())) &&
			(!dst.IsValid() || dst.Contains(e.Dst.Unmap())) &&
			(f.Port == 0 || e.DstPort == f.Port) &&
			(f.Proto == "" || e.Proto == f.Proto) &&
			(f.Rule == "" || e.Rule == f.Rule) &&
			(f.InDev == "" || e.InDev == f.InDev)
	}, nil
}

// addrOrPrefix reads "203.0.113.9" as 203.0.113.9/32, and unmaps and unzones
// first so that ::ffff:203.0.113.9 is the IPv4 address it is.
func addrOrPrefix(s string) (netip.Prefix, error) {
	if p, err := ParseNetwork(s); err == nil {
		return p, nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%q is neither an address nor a network", s)
	}
	a = a.Unmap().WithZone("")
	return netip.PrefixFrom(a, a.BitLen()), nil
}

// PacketLogResult is the reply to GET_PACKET_LOG.
type PacketLogResult struct {
	Entries []PacketLogEntry `json:"entries"` // newest first, at most EffectiveLimit
	Matched int              `json:"matched"` // how many in the ring matched, shown or not
	Held    int              `json:"held"`    // how many the ring holds

	// Listening is whether the core holds its NFLOG group. False with a Reason
	// and Stopped false means the bind never succeeded and the log rules write
	// to the kernel log instead. False with Stopped true means the listener had
	// bound the group and then lost it (a read error after a clean start): the
	// rules still name that group, but nothing reads it any more, so nothing is
	// logged anywhere — not here, not the kernel log — until easywall-core
	// restarts. The two read very differently on /blocked and must not share a
	// sentence.
	Listening bool      `json:"listening"`
	Stopped   bool      `json:"stopped"`
	Group     uint16    `json:"group"`
	Reason    string    `json:"reason,omitempty"`
	Since     time.Time `json:"since,omitempty"` // when the ring began: start, or the oldest replayed entry

	Discarded uint64 `json:"discarded"` // packets too short to decode, and torn lines in the file
	Lost      uint64 `json:"lost"`      // times the socket overran and the kernel dropped messages
	Persisted bool   `json:"persisted"`
}
