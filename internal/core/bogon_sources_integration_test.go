//go:build integration

package core

import (
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// A port rule's private Sources survive the bogon filter.
//
// The filter exempted the allowlist and the Docker networks and nothing else,
// so `tcp 2222 from 10.9.0.0/24` — an operator naming a private network as the
// one allowed in — stopped working the moment the filter was switched on.
// Measured in the harness: bogon off, connected; bogon on, timed out. The
// harness's own addresses are in 10.0.0.0/8, which is the case exactly.
//
// The exemption is as narrow as the rule: the named network, to the named
// port. A port open to anywhere is still closed by the filter to that same
// network, because the operator said nothing about it.
func TestIntegration_ThePortRulesOwnPrivateSourcesPassTheBogonFilter(t *testing.T) {
	h, err := NewHarness()
	if err != nil {
		skipOrFailUnprovable(t, "the harness could not be built: "+err.Error())
	}
	t.Cleanup(h.Close)
	m, err := NewNftablesManagerInNamespace(h.NetNSFd())
	if err != nil {
		skipOrFailUnprovable(t, "no nftables manager for the peer's namespace: "+err.Error())
	}

	const named, anywhere = 2222, 2223
	rules := shared.Rules{
		TCP: []shared.PortRule{
			{Port: "2222", Sources: []string{"# the lab", h.RouterAddr().String() + "/24"}},
			{Port: "2223"},
		},
	}
	dial := func(port uint16) bool {
		t.Helper()
		crossed, err := crossesTo(h.PeerAddr(), port)
		if err != nil {
			t.Fatalf("dialling %d: %v", port, err)
		}
		return crossed
	}

	// The control: with the filter off, both ports answer the router.
	if err := m.Apply(shared.RulesState{Current: rules}, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !dial(named) || !dial(anywhere) {
		skipOrFailUnprovable(t, "with the bogon filter off the open ports did not answer, "+
			"so nothing below would mean anything")
	}

	if err := m.Apply(shared.RulesState{Current: rules}, shared.FirewallOptions{Bogons: true}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !dial(named) {
		t.Errorf("port %d is open to %s/24 and the bogon filter dropped that network anyway — "+
			"the port rule's own Sources must be exempt", named, h.RouterAddr())
	}
	if dial(anywhere) {
		t.Errorf("port %d is open to anywhere and answered a 10.0.0.0/8 source with the bogon filter on — "+
			"the exemption must cover the named port, not the whole network", anywhere)
	}
}
