package core

import (
	"slices"
	"testing"

	"github.com/google/nftables/expr"
	"github.com/jp1337/easywall/internal/shared"
	"golang.org/x/sys/unix"
)

// icmpAcceptTypes builds addEchoAccepts' and addICMPRules' rules and reads
// the accepted types back out, by protocol.
func icmpAcceptTypes(t *testing.T, echo bool, v6 shared.IPv6Config) map[byte][]uint8 {
	t.Helper()
	rec := &recordingConn{}
	m := &NftablesManager{adder: rec}
	tbl := easywallInetTableForTest()
	ch := inputChainForTest(tbl)
	m.addEchoAccepts(tbl, ch, echo, v6)
	m.addICMPRules(tbl, ch, v6)
	got := map[byte][]uint8{}
	for _, r := range rec.rules {
		var cmps []*expr.Cmp
		for _, e := range r.Exprs {
			if c, ok := e.(*expr.Cmp); ok {
				cmps = append(cmps, c)
			}
		}
		if len(cmps) != 2 {
			t.Fatalf("an ICMP rule with %d comparisons; want protocol, then type", len(cmps))
		}
		got[cmps[0].Data[0]] = append(got[cmps[0].Data[0]], cmps[1].Data[0])
	}
	return got
}

var icmpV6Configs = []shared.IPv6Config{
	{Mode: shared.IPv6Filter},
	{Mode: shared.IPv6Filter, ICMPAllowRouterAdvertisement: true},
	{Mode: shared.IPv6Filter, ICMPAllowNeighborAdvertisement: true},
	{Mode: shared.IPv6Filter, ICMPAllowRouterAdvertisement: true, ICMPAllowNeighborAdvertisement: true},
	{Mode: shared.IPv6Block},
	{Mode: shared.IPv6Passthrough},
}

// /blocked says "ICMP type N is not accepted" from shared.ICMPv4Accepted and
// shared.ICMPv6Accepted, and addEchoAccepts with addICMPRules renders the same
// two lists (D7).
// Building the rules and reading the types back keeps it that way, for both
// positions of the switch and every IPv6 configuration.
func TestICMPAcceptsAreTheListsDropReasonReads(t *testing.T) {
	for _, echo := range []bool{false, true} {
		for _, v6 := range icmpV6Configs {
			got := icmpAcceptTypes(t, echo, v6)
			var want6 []uint8
			if v6.Mode == shared.IPv6Filter {
				want6 = shared.ICMPv6Accepted(echo, v6)
			}
			if want4 := shared.ICMPv4Accepted(echo); !slices.Equal(got[unix.IPPROTO_ICMP], want4) {
				t.Errorf("echo=%t %+v: the kernel accepts ICMPv4 %v, /blocked reads %v", echo, v6, got[unix.IPPROTO_ICMP], want4)
			}
			if !slices.Equal(got[unix.IPPROTO_ICMPV6], want6) {
				t.Errorf("echo=%t %+v: the kernel accepts ICMPv6 %v, /blocked reads %v", echo, v6, got[unix.IPPROTO_ICMPV6], want6)
			}
		}
	}
}

// D1, D3, D4, with the types written out rather than read from shared: the
// guard above holds the two lists together, this holds what they say. The
// switch adds exactly the two echo requests, discovery follows its own two
// switches, and the stateless accepts the established,related accept made
// redundant — ICMPv4 0, 3, 11, 12 and ICMPv6 1–4, 129 — never come back.
func TestICMPAcceptsAreEchoAndDiscoveryOnly(t *testing.T) {
	all := shared.IPv6Config{Mode: shared.IPv6Filter, ICMPAllowRouterAdvertisement: true, ICMPAllowNeighborAdvertisement: true}
	for _, tc := range []struct {
		echo         bool
		v6           shared.IPv6Config
		want4, want6 []uint8
	}{
		{false, shared.IPv6Config{Mode: shared.IPv6Filter}, nil, nil},
		{true, shared.IPv6Config{Mode: shared.IPv6Filter}, []uint8{8}, []uint8{128}},
		{false, all, nil, []uint8{133, 134, 135, 136}},
		{true, all, []uint8{8}, []uint8{128, 133, 134, 135, 136}},
		{true, shared.IPv6Config{Mode: shared.IPv6Block}, []uint8{8}, nil},
		{true, shared.IPv6Config{Mode: shared.IPv6Passthrough}, []uint8{8}, nil},
	} {
		got := icmpAcceptTypes(t, tc.echo, tc.v6)
		if !slices.Equal(got[unix.IPPROTO_ICMP], tc.want4) {
			t.Errorf("echo=%t %+v: ICMPv4 accepts %v, want %v", tc.echo, tc.v6, got[unix.IPPROTO_ICMP], tc.want4)
		}
		if !slices.Equal(got[unix.IPPROTO_ICMPV6], tc.want6) {
			t.Errorf("echo=%t %+v: ICMPv6 accepts %v, want %v", tc.echo, tc.v6, got[unix.IPPROTO_ICMPV6], tc.want6)
		}
	}
}
