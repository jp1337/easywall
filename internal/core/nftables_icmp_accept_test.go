package core

import (
	"slices"
	"testing"

	"github.com/google/nftables/expr"
	"github.com/jp1337/easywall/internal/shared"
	"golang.org/x/sys/unix"
)

// /blocked says "ICMP type N is not accepted" from shared.ICMPv4Accepted and
// shared.ICMPv6Accepted; addICMPRules writes the kernel's own list. This builds
// the rules and reads the types back out of them, so the sentence and the
// kernel cannot come to disagree.
func TestICMPAcceptsAreTheListsDropReasonReads(t *testing.T) {
	for _, v6 := range []shared.IPv6Config{
		{Mode: shared.IPv6Filter},
		{Mode: shared.IPv6Filter, ICMPAllowRouterAdvertisement: true},
		{Mode: shared.IPv6Filter, ICMPAllowNeighborAdvertisement: true},
		{Mode: shared.IPv6Filter, ICMPAllowRouterAdvertisement: true, ICMPAllowNeighborAdvertisement: true},
		{Mode: shared.IPv6Block},
	} {
		rec := &recordingConn{}
		m := &NftablesManager{adder: rec}
		tbl := easywallInetTableForTest()
		m.addICMPRules(tbl, inputChainForTest(tbl), v6)
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
		var want6 []uint8
		if v6.Mode == shared.IPv6Filter {
			want6 = shared.ICMPv6Accepted(v6)
		}
		if !slices.Equal(got[unix.IPPROTO_ICMP], shared.ICMPv4Accepted) {
			t.Errorf("%+v: the kernel accepts ICMPv4 %v, /blocked reads %v", v6, got[unix.IPPROTO_ICMP], shared.ICMPv4Accepted)
		}
		if !slices.Equal(got[unix.IPPROTO_ICMPV6], want6) {
			t.Errorf("%+v: the kernel accepts ICMPv6 %v, /blocked reads %v", v6, got[unix.IPPROTO_ICMPV6], want6)
		}
	}
}
