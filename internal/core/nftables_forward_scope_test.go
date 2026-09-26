package core

import (
	"bytes"
	"slices"
	"testing"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"github.com/jp1337/easywall/internal/shared"
	"golang.org/x/sys/unix"
)

// buildForward drives buildForwardChain through the recording adder and returns
// what it wrote. Same seam as TestCheckAcceptsTheNativeCtStateMask uses, and the
// only way this release's ordering can be asserted at all: Apply needs a netlink
// connection, so the order inside it could otherwise only be read.
func buildForward(t *testing.T, docker shared.DockerConfig, routing shared.RoutingConfig,
	rules shared.Rules, cidrs []string) []*nftables.Rule {
	t.Helper()
	got, _ := buildForwardMode(t, docker, routing, rules, cidrs, shared.IPv6Filter)
	return got
}

// buildForwardMode is buildForward with the ipv6 mode named, and the container
// networks buildForwardChain reports it rendered.
func buildForwardMode(t *testing.T, docker shared.DockerConfig, routing shared.RoutingConfig,
	rules shared.Rules, cidrs []string, mode shared.IPv6Mode) ([]*nftables.Rule, []string) {
	t.Helper()
	m := &NftablesManager{}
	rec := &recordingConn{}
	m.adder = rec
	nets := m.buildForwardChain(
		&nftables.Table{Name: tableName, Family: nftables.TableFamilyINet},
		&nftables.Chain{Name: "forward"},
		rules, docker, routing, cidrs, mode)
	return rec.rules, nets
}

// familiesOf returns the nfproto value every rule in rules tests, in order,
// and 0 for a rule that tests none.
func familiesOf(rules []*nftables.Rule) []byte {
	var out []byte
	for _, r := range rules {
		fam, _ := ruleFamily(r.Exprs)
		out = append(out, fam)
	}
	return out
}

func filteredDocker() shared.DockerConfig {
	return shared.DockerConfig{
		Enabled: true, AllowBridgeNetworks: true,
		PublishedPorts: shared.PublishedPortsFiltered,
	}
}

func openDocker() shared.DockerConfig {
	return shared.DockerConfig{
		Enabled: true, AllowBridgeNetworks: true,
		PublishedPorts: shared.PublishedPortsOpen,
	}
}

// indexOfDport returns the position of the first rule matching the given L4
// port, or fails. It fails rather than returning -1 so that a missing rule is
// reported where it is looked for, not as a baffling index comparison.
func indexOfDport(t *testing.T, rules []*nftables.Rule, port uint16) int {
	t.Helper()
	for i, r := range rules {
		if ruleMatchesDport(r, port) {
			return i
		}
	}
	t.Fatalf("no rule in the chain matches port %d", port)
	return -1
}

// indexOfVerdict returns the position of the first rule carrying the given
// verdict kind at or after the given index, or fails.
func indexOfVerdict(t *testing.T, rules []*nftables.Rule, kind expr.VerdictKind, from int) int {
	t.Helper()
	for i := from; i < len(rules); i++ {
		if k, ok := ruleVerdict(rules[i]); ok && k == kind {
			return i
		}
	}
	t.Fatalf("no rule with verdict %v at or after %d (chain has %d rules)", kind, from, len(rules))
	return -1
}

// indexOfCIDRAccept returns the position of the first bridge/routing exception:
// an accept that tests an address against one of the allowed networks and says
// nothing about a port. Found by what it is rather than as "the next accept
// after the deny", so that a chain built in the wrong order reports the order
// rather than a missing rule.
//
// The port clause is not decoration. portAcceptRules builds a rule's sources
// with the same cidrMatch, so a forwarded rule naming one is an accept
// comparing an address too — and the landmark would move to the rule whose
// position is under test.
func indexOfCIDRAccept(t *testing.T, rules []*nftables.Rule) int {
	t.Helper()
	for i, r := range rules {
		k, ok := ruleVerdict(r)
		if !ok || k != expr.VerdictAccept || ruleTestsTransportPort(r) {
			continue
		}
		if ruleComparesAddr(r, posSrcAddr, expr.CmpOpEq) || ruleComparesAddr(r, posDstAddr, expr.CmpOpEq) {
			return i
		}
	}
	t.Fatalf("no CIDR exception in a chain of %d rules", len(rules))
	return -1
}

// THE test of this release. A forwarded rule rendered after the CIDR exception
// can never deny anything, because the exception has already accepted every
// packet with a bridge address at either end — and after Docker's DNAT, an
// inbound packet to a published port has one. A test that only asserted "the
// rule is present" would pass on a ruleset that enforces nothing, which is the
// exact shape of the defect 2.17 was built to convict.
// A rule that names sources is the same rule with a CIDR match in front of it,
// which is the shape that can be confused with the exception it has to precede.
func TestForwardChain_PortRulesComeBeforeTheExceptions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sources []string
	}{
		{"from anywhere", nil},
		{"from a named source", []string{"203.0.113.0/24"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rules := buildForward(t, filteredDocker(), shared.RoutingConfig{Mode: shared.RoutingClosed},
				shared.Rules{TCP: []shared.PortRule{
					{Port: "25", Description: "SMTP", Scope: shared.ScopeForwarded, Sources: tc.sources},
				}},
				[]string{"172.17.0.0/16"})

			accept := indexOfDport(t, rules, 25)
			deny := indexOfVerdict(t, rules, expr.VerdictDrop, accept)
			exception := indexOfCIDRAccept(t, rules)

			if accept >= deny || deny >= exception {
				t.Fatalf("order is accept=%d deny=%d exception=%d; the port rule and the "+
					"deny must both precede the exception, or neither can refuse anything",
					accept, deny, exception)
			}
		})
	}
}

// The established accept has to precede the deny, and this is not symmetry.
// A container's outbound connection comes back with the bridge address as its
// *destination* once conntrack has undone Docker's masquerade — which is
// precisely what the deny matches. Behind the deny, every container on the host
// loses the network at the next apply. It was behind it: addForwardExceptions
// owned the established accept until 2.19, and the exceptions run last.
func TestForwardChain_TheEstablishedAcceptComesBeforeTheDeny(t *testing.T) {
	for _, mode := range []shared.RoutingMode{shared.RoutingClosed, shared.RoutingOpen} {
		rules := buildForward(t, filteredDocker(), shared.RoutingConfig{Mode: mode},
			shared.Rules{}, []string{"172.17.0.0/16"})

		established := -1
		for i, r := range rules {
			if ruleTestsCtState(r) {
				established = i
				break
			}
		}
		if established == -1 {
			t.Fatalf("routing.mode=%q: the forward chain has no established accept at all; "+
				"every container's return traffic is dropped by the deny", mode)
		}
		if deny := indexOfVerdict(t, rules, expr.VerdictDrop, 0); established > deny {
			t.Errorf("routing.mode=%q: established accept at %d, deny at %d — the reply to "+
				"every container's outbound connection is dropped", mode, established, deny)
		}
	}
}

// The direction test is what keeps containers alive. Without it the deny also
// matches a container's own outbound traffic, whose source is the bridge range,
// and every container on the host loses the network at the next apply — the
// failure the acceptance window cannot see, because SSH arrives on input.
func TestForwardChain_TheDenyTestsBothDirections(t *testing.T) {
	rules := buildForward(t, filteredDocker(), shared.RoutingConfig{Mode: shared.RoutingClosed},
		shared.Rules{}, []string{"172.17.0.0/16"})

	deny := rules[indexOfVerdict(t, rules, expr.VerdictDrop, 0)]
	if !ruleComparesAddr(deny, posDstAddr, expr.CmpOpEq) {
		t.Error("the deny does not test the destination against the bridge range")
	}
	if !ruleComparesAddr(deny, posSrcAddr, expr.CmpOpNeq) {
		t.Error("the deny does not exclude sources inside the bridge range, so it " +
			"would drop every container's outbound traffic")
	}

	// The family test comes with each cidrMatch and the rule needs one.
	// `meta nfproto ipv4` printed twice in one line of `nft list ruleset` is
	// output an operator has to read past, and that output is the reason
	// cidrMatch omits a /32 mask.
	var family int
	for _, e := range deny.Exprs {
		if meta, ok := e.(*expr.Meta); ok && meta.Key == expr.MetaKeyNFPROTO {
			family++
		}
	}
	if family != 1 {
		t.Errorf("the deny tests the address family %d times, want 1", family)
	}
}

// published_ports = "open" is the default, and a host upgrading to 2.19 must get
// byte-for-byte what 2.18 gave it. Asserted as "nothing was added", against the
// whole chain, because that is the claim.
func TestForwardChain_OpenRendersExactlyWhatItRenderedBefore(t *testing.T) {
	withRule := buildForward(t, openDocker(), shared.RoutingConfig{Mode: shared.RoutingClosed},
		shared.Rules{TCP: []shared.PortRule{{Port: "25", Scope: shared.ScopeForwarded}}},
		[]string{"172.17.0.0/16"})
	without := buildForward(t, openDocker(), shared.RoutingConfig{Mode: shared.RoutingClosed},
		shared.Rules{}, []string{"172.17.0.0/16"})

	if len(withRule) != len(without) {
		t.Fatalf("under published_ports=open a forwarded rule added %d rule(s)",
			len(withRule)-len(without))
	}
	// And nothing in that chain says no. The count above would still pass if the
	// deny rendered for both, which is the shape that takes a container host off
	// the network without a forwarded rule anywhere in sight.
	for i, r := range withRule {
		if k, ok := ruleVerdict(r); ok && k == expr.VerdictDrop {
			t.Fatalf("under published_ports=open the forward chain carries a drop at %d", i)
		}
	}
}

// A host rule must never reach the forward chain. "both" is the one scope that
// means each, and it is the one that can hide a mistake in either direction.
func TestForwardChain_ScopeDecidesWhatLandsHere(t *testing.T) {
	for _, tc := range []struct {
		scope   shared.PortScope
		present bool
	}{
		{"", false},
		{shared.ScopeHost, false},
		{shared.ScopeForwarded, true},
		{shared.ScopeBoth, true},
	} {
		rules := buildForward(t, filteredDocker(), shared.RoutingConfig{Mode: shared.RoutingClosed},
			shared.Rules{TCP: []shared.PortRule{
				{Port: "25", Scope: tc.scope},
				// A range travels the same road and is rendered by different
				// expressions. Nothing else in this file would notice a forward
				// chain that carried single ports and dropped ranges.
				{Port: "8000:9000", Scope: tc.scope},
			}},
			[]string{"172.17.0.0/16"})
		for _, port := range []uint16{25, 8080} {
			got := false
			for _, r := range rules {
				if ruleMatchesDport(r, port) {
					got = true
				}
			}
			if got != tc.present {
				t.Errorf("scope %q: port %d in the forward chain = %v, want %v",
					tc.scope, port, got, tc.present)
			}
		}
	}
}

// docker.published_ports = "filtered" with no container network detected must
// render nothing at all — not the accepts on their own. The deny is one per
// bridge network, so with no network there is none, and accepts without it open
// in the forward chain exactly the ports 2.18 kept shut while the interface
// reports them enforced. That is this release's own thesis, reproduced by the
// feature.
//
// It is not a configuration error: detection runs at apply, and a host whose
// containers have not started yet legitimately arrives here with none. Config
// refuses only the static contradiction — filtered with docker.enabled = false.
// Both routing modes, because they reach the guard by different doors. Under
// "closed" the chain renders nothing at all and never calls the port builder;
// under "networks" it renders the routing exceptions, so the port builder *is*
// called with an empty bridge list — a host with routing.networks set whose
// containers have not started yet, and the only path on which the accepts could
// still escape without their deny.
func TestForwardChain_FilteredWithNoBridgeRendersNothing(t *testing.T) {
	rules := shared.Rules{TCP: []shared.PortRule{{Port: "25", Scope: shared.ScopeForwarded}}}

	for _, routing := range []shared.RoutingConfig{
		{Mode: shared.RoutingClosed},
		{Mode: shared.RoutingNetworks, Networks: []string{"10.8.0.0/24"}},
	} {
		filtered := buildForward(t, filteredDocker(), routing, rules, nil)
		open := buildForward(t, openDocker(), routing, rules, nil)

		if len(filtered) != len(open) {
			t.Errorf("routing.mode=%q: with no container network, filtered rendered %d rules "+
				"and open rendered %d; they must be the same chain, because the deny that "+
				"gives the accepts meaning cannot render without a network to name",
				routing.Mode, len(filtered), len(open))
		}
		for i, r := range filtered {
			if ruleMatchesDport(r, 25) {
				t.Errorf("routing.mode=%q: rule %d opens port 25 in the forward chain with "+
					"no deny anywhere in it", routing.Mode, i)
			}
		}
	}
}

// routing.mode = "open" skips the exceptions entirely. The forwarded rules must
// render anyway: a key that silently does nothing because another key is set is
// the failure mode this release exists to end.
func TestForwardChain_RoutingOpenStillRendersTheForwardedRules(t *testing.T) {
	rules := buildForward(t, filteredDocker(), shared.RoutingConfig{Mode: shared.RoutingOpen},
		shared.Rules{TCP: []shared.PortRule{{Port: "25", Scope: shared.ScopeForwarded}}},
		[]string{"172.17.0.0/16"})
	found := false
	for _, r := range rules {
		if ruleMatchesDport(r, 25) {
			found = true
		}
	}
	if !found {
		t.Error("routing.mode=open dropped the forwarded port rule")
	}
}

// --- predicates over a built rule -------------------------------------------

// ruleMatchesDport reports whether the rule's L4 destination port test covers
// the given port. The payload load has to be found first: the same two bytes at
// a different offset are a source port, and the same comparison against a
// network offset is an address.
//
// Ranges count. buildPortExprs emits gte/lte for "8000:9000" and a predicate
// that only understood the single-port equality would report every range rule
// as absent — so a forward chain that silently dropped ranges would pass every
// test in this file.
func ruleMatchesDport(r *nftables.Rule, port uint16) bool {
	want := portBytes(int(port))
	for i, e := range r.Exprs {
		p, isPayload := e.(*expr.Payload)
		if !isPayload || p.Base != expr.PayloadBaseTransportHeader || p.Offset != 2 || p.Len != 2 {
			continue
		}
		if i+1 >= len(r.Exprs) {
			return false
		}
		first, isCmp := r.Exprs[i+1].(*expr.Cmp)
		if !isCmp {
			continue
		}
		switch first.Op {
		case expr.CmpOpEq:
			if bytes.Equal(first.Data, want) {
				return true
			}
		case expr.CmpOpGte:
			// Both bounds are two bytes, most significant first — the wire order
			// portBytes packs — so the bytewise comparison is the numeric one.
			if i+2 >= len(r.Exprs) {
				continue
			}
			second, ok := r.Exprs[i+2].(*expr.Cmp)
			if ok && second.Op == expr.CmpOpLte &&
				bytes.Compare(want, first.Data) >= 0 && bytes.Compare(want, second.Data) <= 0 {
				return true
			}
		}
	}
	return false
}

// ruleTestsTransportPort reports whether the rule looks at an L4 port at all.
// It is what separates a CIDR exception from a port rule that names sources:
// both are accepts comparing an address against a network.
func ruleTestsTransportPort(r *nftables.Rule) bool {
	for _, e := range r.Exprs {
		if p, ok := e.(*expr.Payload); ok && p.Base == expr.PayloadBaseTransportHeader {
			return true
		}
	}
	return false
}

// ruleVerdict returns the rule's trailing verdict. The second return is not
// decoration: expr.VerdictDrop is the zero value of expr.VerdictKind, so a
// predicate that reported "no verdict" as the zero value would report every
// rule that issues none as a drop.
func ruleVerdict(r *nftables.Rule) (expr.VerdictKind, bool) {
	if len(r.Exprs) == 0 {
		return 0, false
	}
	v, ok := r.Exprs[len(r.Exprs)-1].(*expr.Verdict)
	if !ok {
		return 0, false
	}
	return v.Kind, true
}

// ruleComparesAddr reports whether the rule tests the address at the given
// position with the given operator — the two halves of the deny's direction
// test, which is the only thing keeping a container's own traffic out of it.
func ruleComparesAddr(r *nftables.Rule, pos addrPos, op expr.CmpOp) bool {
	for i, e := range r.Exprs {
		p, isPayload := e.(*expr.Payload)
		if !isPayload || p.Base != expr.PayloadBaseNetworkHeader {
			continue
		}
		switch {
		case p.Offset == pos.v4 && p.Len == 4:
		case p.Offset == pos.v6 && p.Len == 16:
		default:
			continue
		}
		// The comparison may sit behind a mask for a network narrower than a
		// single address; the first Cmp after the load is the address test.
		for _, after := range r.Exprs[i+1:] {
			if c, isCmp := after.(*expr.Cmp); isCmp {
				return c.Op == op
			}
		}
	}
	return false
}

// ruleTestsCtState reports whether the rule reads conntrack state — which, in
// this chain, only the established accept does.
func ruleTestsCtState(r *nftables.Rule) bool {
	for _, e := range r.Exprs {
		if c, ok := e.(*expr.Ct); ok && c.Key == expr.CtKeySTATE {
			return true
		}
	}
	return false
}

// A forwarded accept with no sources must still name an address family.
//
// The table is inet, so a rule that tests only `meta l4proto` and a port
// matches IPv4 and IPv6 alike. The deny it is paired with is built from
// detectDockerBridges, which returns IPv4 CIDRs only — so an unpinned accept
// opens the port for forwarded IPv6 to anything the host routes, well past the
// containers the rule is about, with nothing below able to refuse it. 2.18's
// policy drop refused exactly that traffic.
//
// A rule naming a source already carries the family test cidrMatch put there,
// and must not carry a second one: `meta nfproto ipv4` twice in one line of
// `nft list ruleset` is output an operator has to read past.
func TestForwardChain_TheAcceptNamesAnAddressFamily(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sources []string
		want    byte
	}{
		{"from anywhere", nil, unix.NFPROTO_IPV4},
		{"from a named IPv4 source", []string{"203.0.113.0/24"}, unix.NFPROTO_IPV4},
		{"from a named IPv6 source", []string{"2001:db8::/32"}, unix.NFPROTO_IPV6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rules := buildForward(t, filteredDocker(), shared.RoutingConfig{Mode: shared.RoutingClosed},
				shared.Rules{TCP: []shared.PortRule{
					{Port: "25", Scope: shared.ScopeForwarded, Sources: tc.sources},
				}},
				[]string{"172.17.0.0/16"})

			accept := rules[indexOfDport(t, rules, 25)]
			var families []byte
			for i, e := range accept.Exprs {
				meta, ok := e.(*expr.Meta)
				if !ok || meta.Key != expr.MetaKeyNFPROTO {
					continue
				}
				cmp, ok := accept.Exprs[i+1].(*expr.Cmp)
				if !ok || len(cmp.Data) != 1 {
					t.Fatalf("the family test is not followed by a one-byte comparison")
				}
				families = append(families, cmp.Data[0])
			}
			if len(families) != 1 || families[0] != tc.want {
				t.Errorf("the accept tests nfproto %v, want exactly [%d] — an accept that "+
					"names no family opens the port for forwarded IPv6 as well, and the "+
					"deny beside it is IPv4-only", families, tc.want)
			}
		})
	}
}

// D1: an IPv6 container network gets the forwarded accept a second time, in
// its own family, and both come before the first deny.
func TestForwardChain_AnIPv6ContainerNetworkGetsATwin(t *testing.T) {
	rules, nets := buildForwardMode(t, filteredDocker(), shared.RoutingConfig{Mode: shared.RoutingClosed},
		shared.Rules{TCP: []shared.PortRule{{ID: "aaaaaaaaaaa1", Port: "25", Scope: shared.ScopeForwarded}}},
		[]string{"172.17.0.0/16", "fd00:ea5e::/64"}, shared.IPv6Filter)

	var accepts []byte
	for _, r := range rules {
		if ruleMatchesDport(r, 25) {
			fam, _ := ruleFamily(r.Exprs)
			accepts = append(accepts, fam)
		}
	}
	if !slices.Equal(accepts, []byte{unix.NFPROTO_IPV4, unix.NFPROTO_IPV6}) {
		t.Fatalf("port 25 renders with families %v, want one IPv4 and one IPv6 accept", accepts)
	}
	firstDrop := indexOfVerdict(t, rules, expr.VerdictDrop, 0)
	for i, r := range rules {
		if ruleMatchesDport(r, 25) && i > firstDrop {
			t.Errorf("an accept for port 25 sits behind the deny at %d and can never be reached", firstDrop)
		}
	}
	var drops []byte
	for _, r := range rules {
		if k, ok := ruleVerdict(r); ok && k == expr.VerdictDrop {
			fam, _ := ruleFamily(r.Exprs)
			drops = append(drops, fam)
		}
	}
	if !slices.Equal(drops, []byte{unix.NFPROTO_IPV4, unix.NFPROTO_IPV6}) {
		t.Errorf("the per-bridge denies carry families %v, want one per network", drops)
	}
	// The IPv6 bridge's own exceptions (spec §5): an accept testing an address
	// in the IPv6 family and no port.
	v6Exception := false
	for _, r := range rules {
		if k, ok := ruleVerdict(r); ok && k == expr.VerdictAccept && !ruleTestsTransportPort(r) {
			if fam, _ := ruleFamily(r.Exprs); fam == unix.NFPROTO_IPV6 {
				v6Exception = true
			}
		}
	}
	if !v6Exception {
		t.Error("the IPv6 container network got no exception: its containers could not reach anything")
	}
	if !slices.Equal(nets, []string{"172.17.0.0/16", "fd00:ea5e::/64"}) {
		t.Errorf("buildForwardChain reports it rendered %v", nets)
	}
}

// A host whose only container network is IPv6 gets only the IPv6 copy: an
// IPv4 accept would open the port for forwarded IPv4 with no IPv4 deny beside it.
func TestForwardChain_OnlyAnIPv6NetworkRendersOnlyTheIPv6Copy(t *testing.T) {
	rules := buildForward(t, filteredDocker(), shared.RoutingConfig{Mode: shared.RoutingClosed},
		shared.Rules{TCP: []shared.PortRule{{Port: "25", Scope: shared.ScopeForwarded}}},
		[]string{"fd00:ea5e::/64"})
	for _, r := range rules {
		if ruleMatchesDport(r, 25) {
			if fam, _ := ruleFamily(r.Exprs); fam != unix.NFPROTO_IPV6 {
				t.Errorf("port 25 renders an accept in family %d on a host with only an IPv6 container network", fam)
			}
		}
	}
}

// D3: under block, nothing IPv6 renders in the forward chain for containers —
// not the detected network, not a custom one, not the forwarded rule's twin,
// not a forwarded rule's own IPv6 source — and the rendered networks say so.
// (routing.networks is not a container network and is not part of D3.)
func TestForwardChain_BlockKeepsIPv6AwayFromContainers(t *testing.T) {
	docker := filteredDocker()
	docker.CustomNetworks = []string{"2001:db8:c0::/64"}
	cidrs := containerNetworks(docker, []string{"172.17.0.0/16", "fd00:ea5e::/64"}, shared.IPv6Block)
	rules, nets := buildForwardMode(t, docker, shared.RoutingConfig{Mode: shared.RoutingClosed},
		shared.Rules{TCP: []shared.PortRule{
			{Port: "25", Scope: shared.ScopeForwarded},
			{Port: "587", Scope: shared.ScopeForwarded, Sources: []string{"2001:db8::/32", "203.0.113.0/24"}},
		}},
		cidrs, shared.IPv6Block)

	for i, fam := range familiesOf(rules) {
		if fam == unix.NFPROTO_IPV6 {
			t.Errorf("rule %d tests IPv6 under ipv6.mode = block: %#v", i, rules[i].Exprs)
		}
	}
	if !slices.Equal(nets, []string{"172.17.0.0/16"}) {
		t.Errorf("under block buildForwardChain reports it rendered %v, want only the IPv4 network", nets)
	}
	// The IPv4 half of the same rule is untouched.
	found := false
	for _, r := range rules {
		if ruleMatchesDport(r, 587) {
			found = true
		}
	}
	if !found {
		t.Error("the IPv4 source of a mixed forwarded rule disappeared with its IPv6 one")
	}
}

// D2's second named exception: a container-network list with no usable network
// (only a comment, or an entry that does not parse) rendered, in 2.23.2,
// forwarded accepts with no deny beside them — a port opened for forwarded
// traffic to anything the host routes. Now nothing renders, not even the
// established accept: forwardPortRulesRender and addForwardPortRules read the
// same cidrFamilies question, so a list that is non-empty but has nothing
// usable is the same case as no list at all, and buildForwardChain's own
// guard (len(exceptions) == 0 too, since forwardExceptionMatches skips the
// same entries) never gets past its own early return. Not in the golden,
// which is 2.23.2's behaviour.
func TestForwardChain_AListWithNoUsableNetworkRendersNoAccept(t *testing.T) {
	rules := buildForward(t, filteredDocker(), shared.RoutingConfig{Mode: shared.RoutingClosed},
		shared.Rules{TCP: []shared.PortRule{{Port: "25", Scope: shared.ScopeForwarded}}},
		[]string{"# only a note", "not-a-cidr"})
	if len(rules) != 0 {
		t.Fatalf("a list with no usable network rendered %d rule(s), want an empty chain: %#v",
			len(rules), rules)
	}
}

// Review Focus 2: the twin is the same UI rule, so it carries the same id —
// the counters and "last used" read both kernel rules as one.
//
// Equal is not enough. portAcceptRules is called once per copy specifically
// so that no two kernel rules share an expression or a userdata backing
// array — a mutation that instead derived the IPv6 twin by re-pinning the
// IPv4 copy's own Exprs into a new slice would still carry an equal-by-value
// id and pass a bytes.Equal check, while the two "rules" were, underneath,
// one and the same kernel object: writing one through the netlink layer, or
// truncating its slice, would corrupt the other.
func TestForwardChain_TheTwinCarriesTheRulesID(t *testing.T) {
	rules := buildForward(t, filteredDocker(), shared.RoutingConfig{Mode: shared.RoutingClosed},
		shared.Rules{TCP: []shared.PortRule{{ID: "aaaaaaaaaaa1", Port: "25", Scope: shared.ScopeForwarded}}},
		[]string{"172.17.0.0/16", "fd00:ea5e::/64"})
	var twins []*nftables.Rule
	for _, r := range rules {
		if ruleMatchesDport(r, 25) {
			twins = append(twins, r)
		}
	}
	if len(twins) != 2 {
		t.Fatalf("port 25 rendered %d matching rule(s), want 2", len(twins))
	}
	tag0, tag1 := twins[0].UserData, twins[1].UserData
	if !bytes.Equal(tag0, tag1) || len(tag0) == 0 {
		t.Errorf("the two copies of one forwarded rule carry userdata %x; both must carry the rule's id", [][]byte{tag0, tag1})
	}
	if len(tag0) > 0 && len(tag1) > 0 && &tag0[0] == &tag1[0] {
		t.Error("the two copies' userdata share a backing array: writing one through " +
			"the netlink layer would corrupt the other")
	}
	for _, e0 := range twins[0].Exprs {
		for _, e1 := range twins[1].Exprs {
			if e0 == e1 {
				t.Errorf("the two copies share an expression by pointer identity (%p): "+
					"they are not two kernel rules but one rule aliased twice", e0)
			}
		}
	}
}
