package core

import (
	"bytes"
	"testing"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"github.com/jp1337/easywall/internal/shared"
)

// buildForward drives buildForwardChain through the recording adder and returns
// what it wrote. Same seam as TestCheckAcceptsTheNativeCtStateMask uses, and the
// only way this release's ordering can be asserted at all: Apply needs a netlink
// connection, so the order inside it could otherwise only be read.
func buildForward(t *testing.T, docker shared.DockerConfig, routing shared.RoutingConfig,
	rules shared.Rules, cidrs []string) []*nftables.Rule {
	t.Helper()
	m := &NftablesManager{}
	rec := &recordingConn{}
	m.adder = rec
	m.buildForwardChain(
		&nftables.Table{Name: tableName, Family: nftables.TableFamilyINet},
		&nftables.Chain{Name: "forward"},
		rules, docker, routing, cidrs)
	return rec.rules
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
// an accept that tests an address against one of the allowed networks. Found by
// what it is rather than as "the next accept after the deny", so that a chain
// built in the wrong order reports the order rather than a missing rule.
func indexOfCIDRAccept(t *testing.T, rules []*nftables.Rule) int {
	t.Helper()
	for i, r := range rules {
		k, ok := ruleVerdict(r)
		if !ok || k != expr.VerdictAccept {
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
func TestForwardChain_PortRulesComeBeforeTheExceptions(t *testing.T) {
	rules := buildForward(t, filteredDocker(), shared.RoutingConfig{Mode: shared.RoutingClosed},
		shared.Rules{TCP: []shared.PortRule{
			{Port: "25", Description: "SMTP", Scope: shared.ScopeForwarded},
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
			shared.Rules{TCP: []shared.PortRule{{Port: "25", Scope: tc.scope}}},
			[]string{"172.17.0.0/16"})
		got := false
		for _, r := range rules {
			if ruleMatchesDport(r, 25) {
				got = true
			}
		}
		if got != tc.present {
			t.Errorf("scope %q: port 25 in the forward chain = %v, want %v",
				tc.scope, got, tc.present)
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

// ruleMatchesDport reports whether the rule carries an equality test on the L4
// destination port. The payload load has to be found first: the same two bytes
// at a different offset are a source port, and the same comparison against a
// network offset is an address.
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
		c, isCmp := r.Exprs[i+1].(*expr.Cmp)
		if isCmp && c.Op == expr.CmpOpEq && bytes.Equal(c.Data, want) {
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
