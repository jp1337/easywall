package shared

import (
	"fmt"
	"net/netip"
	"reflect"
	"testing"
	"time"
)

// The rules a default-drop row is read against. Each entry is here for one
// case below; the comment says which.
var whyRules = Rules{
	TCP: []PortRule{
		{Port: "22"},
		{Port: "8443", Sources: []string{"10.0.0.0/8", "# the office"}}, // open only for others; a comment is not a source
		{Port: "5432", Sources: []string{"# not finished"}},             // comments only: no rule at all
		{Port: "9000", Scope: ScopeForwarded},                           // reaches the forward chain only
		{Port: "6000:6010"},                                             // a range
		{Port: "25", Sources: []string{"2001:db8:1::/48"}},              // an IPv6 source opens nothing to IPv4
	},
	UDP:       []PortRule{{Port: "53"}},
	Blocklist: []string{"192.0.2.66"},
	Allowlist: []string{"# admins", "198.51.100.7", "192.0.2.9"}, // .9: allowlisted as IPv4 only, for the mapped-source case below
}

func whyTCP(src string, port uint16) PacketLogEntry {
	a := netip.MustParseAddr(src)
	fam := uint8(4)
	if a.Is6() {
		fam = 6
	}
	return PacketLogEntry{Rule: "drop", Hook: "input", Family: fam, Proto: "tcp", Src: a, DstPort: port}
}

func whyICMP(proto string, typ uint8) PacketLogEntry {
	e := whyTCP("203.0.113.9", 0)
	e.Proto, e.ICMP = proto, &PacketICMP{Type: typ}
	if proto == "icmpv6" {
		e.Family, e.Src = 6, netip.MustParseAddr("2001:db8::9")
	}
	return e
}

func edited(e PacketLogEntry, f func(*PacketLogEntry)) PacketLogEntry { f(&e); return e }

var (
	v6Filter      = NetworkSettings{IPv6: IPv6Config{Mode: IPv6Filter}}
	v6FilterRA    = NetworkSettings{IPv6: IPv6Config{Mode: IPv6Filter, ICMPAllowRouterAdvertisement: true}}
	v6Block       = NetworkSettings{IPv6: IPv6Config{Mode: IPv6Block}}
	v6Passthrough = NetworkSettings{IPv6: IPv6Config{Mode: IPv6Passthrough}}
	v6DockerNet   = NetworkSettings{IPv6: IPv6Config{Mode: IPv6Filter},
		Docker: DockerConfig{Enabled: true, CustomNetworks: []string{"172.20.0.0/16"}}}
)

var dropReasonCases = []struct {
	name string
	e    PacketLogEntry
	n    NetworkSettings
	want DropWhy
}{
	{"no rule for the port", whyTCP("203.0.113.9", 993), v6Filter,
		DropWhy{DropPortClosed, map[string]any{"Port": uint16(993), "Proto": "tcp"}}},
	{"a TCP rule opens nothing for UDP", edited(whyTCP("203.0.113.9", 22), func(e *PacketLogEntry) { e.Proto = "udp" }), v6Filter,
		DropWhy{DropPortClosed, map[string]any{"Port": uint16(22), "Proto": "udp"}}},
	{"open now", whyTCP("203.0.113.9", 22), v6Filter,
		DropWhy{DropPortOpenNow, map[string]any{"Port": uint16(22), "Proto": "tcp"}}},
	{"open now, inside a range", whyTCP("203.0.113.9", 6005), v6Filter,
		DropWhy{DropPortOpenNow, map[string]any{"Port": uint16(6005), "Proto": "tcp"}}},
	{"open only for other sources", whyTCP("203.0.113.9", 8443), v6Filter,
		DropWhy{DropPortSources, map[string]any{"Port": uint16(8443), "Proto": "tcp", "Sources": "10.0.0.0/8"}}},
	{"open now for this source", whyTCP("10.1.2.3", 8443), v6Filter,
		DropWhy{DropPortOpenNow, map[string]any{"Port": uint16(8443), "Proto": "tcp"}}},
	{"a source list of comments opens nothing", whyTCP("203.0.113.9", 5432), v6Filter,
		DropWhy{DropPortClosed, map[string]any{"Port": uint16(5432), "Proto": "tcp"}}},
	{"forwarded only", whyTCP("203.0.113.9", 9000), v6Filter,
		DropWhy{DropPortForwarded, map[string]any{"Port": uint16(9000), "Proto": "tcp"}}},
	{"an IPv6 source does not open the port to IPv4", whyTCP("203.0.113.9", 25), v6Filter,
		DropWhy{DropPortSources, map[string]any{"Port": uint16(25), "Proto": "tcp", "Sources": "2001:db8:1::/48"}}},
	{"the same rule, from its IPv6 network", whyTCP("2001:db8:1::5", 25), v6Filter,
		DropWhy{DropPortOpenNow, map[string]any{"Port": uint16(25), "Proto": "tcp"}}},
	{"blocklisted since", whyTCP("192.0.2.66", 22), v6Filter, DropWhy{Code: DropBlocklistedNow}},
	{"allowlisted since", whyTCP("198.51.100.7", 993), v6Filter, DropWhy{Code: DropAllowlistedNow}},
	{"an IPv4 ping", whyICMP("icmp", 8), v6Filter, DropWhy{Code: DropIPv4Ping}},
	{"an ICMP type nothing accepts", whyICMP("icmp", 13), v6Filter,
		DropWhy{DropICMPType, map[string]any{"Proto": "ICMP", "Type": uint8(13)}}},
	{"an accepted ICMP type", whyICMP("icmp", 3), v6Filter,
		DropWhy{DropICMPAcceptedNow, map[string]any{"Proto": "ICMP", "Type": uint8(3)}}},
	{"a 2.21 ICMP line, no type", edited(whyICMP("icmp", 8), func(e *PacketLogEntry) { e.ICMP = nil }), v6Filter,
		DropWhy{DropICMPUntyped, map[string]any{"Proto": "ICMP"}}},
	{"router advertisement, discovery off", whyICMP("icmpv6", 134), v6Filter,
		DropWhy{DropICMPType, map[string]any{"Proto": "ICMPv6", "Type": uint8(134)}}},
	{"router advertisement, discovery on now", whyICMP("icmpv6", 134), v6FilterRA,
		DropWhy{DropICMPAcceptedNow, map[string]any{"Proto": "ICMPv6", "Type": uint8(134)}}},
	{"an IPv6 ping is accepted", whyICMP("icmpv6", 128), v6Filter,
		DropWhy{DropICMPAcceptedNow, map[string]any{"Proto": "ICMPv6", "Type": uint8(128)}}},
	{"a ping from an allowlisted source: the allowlist is the step", edited(whyICMP("icmp", 8), func(e *PacketLogEntry) {
		e.Src = netip.MustParseAddr("198.51.100.7")
	}), v6Filter, DropWhy{Code: DropAllowlistedNow}},
	{"a protocol without ports", edited(whyTCP("203.0.113.9", 0), func(e *PacketLogEntry) { e.Proto = "47" }), v6Filter,
		DropWhy{Code: DropNoPort}},
	{"a later fragment: no port was read", whyTCP("203.0.113.9", 0), v6Filter, DropWhy{Code: DropNoPort}},
	{"IPv6 is blocked now", whyTCP("2001:db8::9", 22), v6Block, DropWhy{Code: DropIPv6Blocked}},
	{"IPv6 is blocked now, before any ICMP accept", whyICMP("icmpv6", 128), v6Block, DropWhy{Code: DropIPv6Blocked}},
	{"IPv6 is passed through now", whyTCP("2001:db8::9", 993), v6Passthrough, DropWhy{Code: DropIPv6Passthrough}},
	{"the IPv6 mode does not touch IPv4", whyTCP("203.0.113.9", 993), v6Block,
		DropWhy{DropPortClosed, map[string]any{"Port": uint16(993), "Proto": "tcp"}}},
	{"a module row names itself", edited(whyTCP("203.0.113.9", 22), func(e *PacketLogEntry) { e.Rule = "ssh" }), v6Filter, DropWhy{}},
	{"no easywall rule logs in the forward chain", edited(whyTCP("203.0.113.9", 993), func(e *PacketLogEntry) { e.Hook = "forward" }), v6Filter, DropWhy{}},
	{"an IPv4-mapped source in an IPv6 packet is not read as the IPv4 allowlist entry",
		edited(whyTCP("203.0.113.9", 993), func(e *PacketLogEntry) {
			e.Family, e.Src = 6, netip.MustParseAddr("::ffff:192.0.2.9")
		}), v6Filter,
		DropWhy{DropPortClosed, map[string]any{"Port": uint16(993), "Proto": "tcp"}}},
	{"a Docker custom network gets no reason", whyTCP("172.20.5.9", 993), v6DockerNet, DropWhy{}},
}

func TestDropReasonWalksTheChainInOrder(t *testing.T) {
	for _, tc := range dropReasonCases {
		if got := tc.e.DropReason(whyRules, tc.n); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// Every code is reachable, and every code reached is in AllDropReasons — the
// list the locale guard hangs off.
func TestDropReasonReachesEveryCode(t *testing.T) {
	seen := map[DropReason]bool{}
	for _, tc := range dropReasonCases {
		seen[tc.e.DropReason(whyRules, tc.n).Code] = true
	}
	delete(seen, "")
	for _, c := range AllDropReasons {
		if !seen[c] {
			t.Errorf("%q is in AllDropReasons and no case reaches it", c)
		}
		delete(seen, c)
	}
	for c := range seen {
		t.Errorf("%q is produced but missing from AllDropReasons, so no locale guard covers it", c)
	}
}

// The realistic upper bound: /import accepts 512 KiB (handler_export.go:33),
// which holds about 10,000 short entries. Before ParseRules existed,
// DropReason reparsed a list this size on every InAnyEntry call — up to three
// times per row — and /blocked/rows asks that of every on-screen row, every
// five-second poll; measured at 197ms for 200 rows at 10,000 entries and 1.02s
// at 50,000. A loop that re-parses per row still finishes under a second at
// this size — the wall-clock timer alone does not carry the test — so what
// actually proves parsing runs once is allocations: DropReasonParsed on an
// already-parsed ParsedRules allocates a small, list-size-independent
// constant (measured at 4 on this row: the Port/Proto map and the loop
// variable, nothing that scales with the blocklist); re-parsing per call
// would allocate a netip.Prefix slice sized to the list on every one of the
// 100 runs AllocsPerRun makes. The timer stays as documentation of the
// user-facing cost.
func TestParseRulesOnceHoldsA10000EntryList(t *testing.T) {
	entries := make([]string, 10000)
	for i := range entries {
		entries[i] = fmt.Sprintf("10.%d.%d.%d/32", i/65536%256, i/256%256, i%256)
	}
	big := whyRules
	big.Blocklist = entries
	parsed := ParseRules(big)
	row := whyTCP("203.0.113.9", 993)

	const maxAllocs = 8 // measured 4; margin for the map plus a small slice, never for list size
	if n := testing.AllocsPerRun(10, func() { row.DropReasonParsed(parsed, v6Filter) }); n > maxAllocs {
		t.Errorf("DropReasonParsed allocated %v times per call against a 10,000-entry list; want <= %d, independent of list size", n, maxAllocs)
	}

	start := time.Now()
	for i := 0; i < 200; i++ { // a realistic on-screen row count
		row.DropReasonParsed(parsed, v6Filter)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("200 rows against a 10,000-entry blocklist took %s parsed once; want well under 1s", elapsed)
	}
}
