package core

import (
	"reflect"
	"testing"

	"github.com/google/nftables/expr"
)

// An IPv4-mapped network builds exactly what the IPv4 network it names builds,
// in every builder that turns a list entry into a match. Before 2.22 each of
// them paired a 16-byte mask with a 4-byte compare (or, in cidrMatch, returned
// nil): EINVAL for the whole rule set, or a port-rule source silently skipped.
//
// 10.0.0.0/08 is here for the parser: net.ParseCIDR accepts it, so validation
// does, and a builder that parsed with netip.ParsePrefix would drop it.
func TestAMappedNetworkBuildsWhatItsIPv4NetworkBuilds(t *testing.T) {
	tbl := easywallInetTableForTest()
	ch := inputChainForTest(tbl)
	added := func(add func(m *NftablesManager, entry string)) func(string) []expr.Any {
		return func(entry string) []expr.Any {
			rec := &recordingConn{}
			add(&NftablesManager{adder: rec}, entry)
			if len(rec.rules) != 1 {
				return nil
			}
			return rec.rules[0].Exprs
		}
	}
	builders := []struct {
		name  string
		build func(string) []expr.Any
	}{
		{"ipv4SourceMatch", ipv4SourceMatch},
		{"cidrMatch", func(s string) []expr.Any { return cidrMatch(s, posSrcAddr) }},
		{"cidrMatchNegated", func(s string) []expr.Any { return cidrMatchNegated(s, posDstAddr) }},
		{"addCIDRAccept", added(func(m *NftablesManager, s string) { m.addCIDRAccept(tbl, ch, s) })},
		{"cidrDropMatch", cidrDropMatch},
	}
	for _, b := range builders {
		want := b.build("10.0.0.0/8")
		if want == nil {
			t.Fatalf("%s built nothing for 10.0.0.0/8", b.name)
		}
		for _, in := range []string{"::ffff:10.0.0.0/104", "::FFFF:10.9.8.7/104", "10.0.0.0/08"} {
			if got := b.build(in); !reflect.DeepEqual(got, want) {
				t.Errorf("%s(%q) does not build the expressions of 10.0.0.0/8", b.name, in)
			}
		}
	}
}
