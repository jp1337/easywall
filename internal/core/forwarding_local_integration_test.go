//go:build integration

package core

import (
	"strconv"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// Port forwarding redirects only what is addressed to this host.
//
// forwarding.md promises it ("Not for traffic passing through"), and the
// prerouting rule said nothing of the kind: a bare `tcp dport 80 redirect to
// :8080`, so on a host that routes, a client dialling some other machine's
// port 80 through it was answered by this host's 8080. Measured before the fix
// with the harness: the peer routes for the router, the router dials the
// container's port 80, and the redirect captured it.
//
// The two tables make each outcome unambiguous. In the first, 8080 is closed
// and routing lets the container network across, so a SYN that crosses can
// only have been forwarded to the container — a redirect would meet the
// input chain's drop. In the second, 8080 is open and 80 is not, so a
// connection to the peer's own port 80 crosses only through the redirect.
func TestIntegration_ForwardingRedirectsOnlyTrafficForThisHost(t *testing.T) {
	h, m := forwardScopeHarness(t)
	fwd := []shared.ForwardingRule{{Protocol: "tcp", SourcePort: 80, DestPort: selftestForwardedPort}}

	routed := shared.NetworkSettings{Routing: shared.RoutingConfig{
		Mode:     shared.RoutingNetworks,
		Networks: []string{h.BridgeCIDR()},
	}}
	// The control: the same table without the forward carries the routed
	// connection, so a timeout below is the redirect and not the routing.
	if err := m.Apply(shared.RulesState{}, shared.FirewallOptions{}, routed); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if crossed, err := crossesTo(h.ContainerAddr(), 80); err != nil || !crossed {
		skipOrFailUnprovable(t, "with no forwarding rule the routed connection did not cross, "+
			"so nothing below would mean anything")
	}

	if err := m.Apply(shared.RulesState{Current: shared.Rules{Forwarding: fwd}},
		shared.FirewallOptions{}, routed); err != nil {
		t.Fatalf("apply: %v", err)
	}
	crossed, err := crossesTo(h.ContainerAddr(), 80)
	if err != nil {
		t.Fatalf("dialling the container through the peer: %v", err)
	}
	if !crossed {
		t.Errorf("a connection to %s:80, routed through the peer, was captured by the peer's "+
			"forwarding rule and redirected to its own port %d — forwarding must touch only "+
			"packets addressed to this host", h.ContainerAddr(), selftestForwardedPort)
	}

	local := shared.Rules{
		Forwarding: fwd,
		TCP:        []shared.PortRule{{Port: strconv.Itoa(selftestForwardedPort)}},
	}
	if err := m.Apply(shared.RulesState{Current: local}, shared.FirewallOptions{}, routed); err != nil {
		t.Fatalf("apply: %v", err)
	}
	crossed, err = crossesTo(h.PeerAddr(), 80)
	if err != nil {
		t.Fatalf("dialling the peer: %v", err)
	}
	if !crossed {
		t.Errorf("a connection to the peer's own port 80 was not redirected to %d", selftestForwardedPort)
	}
}
