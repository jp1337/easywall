//go:build integration

package core

// The forward chain's two directions, measured with real packets through a real
// container leg.
//
// The rendering tests in nftables_forward_scope_test.go prove the order in
// easywall's own model of the ruleset, which is not the same as proving it
// filters. These two tests are the other half, and between them they cover the
// two ways 2.19 can destroy a production host — neither of which the 120-second
// acceptance window can see, because the operator's SSH session arrives on the
// *input* chain and stays up through both:
//
//	inbound   a forwarded rule rendered after the CIDR exceptions can never deny
//	          anything, so every published port stays open while the interface
//	          reports it filtered.
//	outbound  the deny matches the shape of a reply to a container's own
//	          outbound connection — dst in the bridge, src outside it, which is
//	          what conntrack hands back once Docker's masquerade is undone. With
//	          no state rule in front of it, every container on the host loses
//	          the network at the next apply.
//
// The second is the one nobody would think to write and the one that matters
// most, and it is the only test in this package that opens a connection *from*
// the container.

import (
	"net"
	"strconv"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The listener this side of the peer, for the outbound direction. A real
// handshake and not a RST, because "the container can still reach the world" is
// the claim and a refusal would not settle it.
const forwardScopeListenPort = 12236

// forwardScopeHarness builds the three namespaces and a manager bound to the
// middle one, and gives up honestly when the host will not carry them.
//
// The table goes into the peer's namespace and nowhere else — the same
// arrangement the self-test uses — so nothing here touches the table the suite's
// own namespace is holding.
func forwardScopeHarness(t *testing.T) (*Harness, *NftablesManager) {
	t.Helper()

	h, err := NewHarness()
	if err != nil {
		skipOrFailUnprovable(t, "the harness could not be built: "+err.Error())
	}
	t.Cleanup(h.Close)

	if err := h.AddContainerLeg(); err != nil {
		skipOrFailUnprovable(t, "the container leg could not be built: "+err.Error())
	}

	m, err := NewNftablesManagerInNamespace(h.NetNSFd())
	if err != nil {
		skipOrFailUnprovable(t, "no nftables manager for the peer's namespace: "+err.Error())
	}
	return h, m
}

// forwardScopeTable is the configuration under test: filtering on, the
// container's own network named, and one forwarded rule for one port.
//
// custom_networks rather than a detected bridge, because detection reads this
// host's Docker and there is none in the suite's namespace. The rendering path
// is identical — Apply concatenates the detected and the configured ranges into
// one list before buildForwardChain ever sees them.
func forwardScopeTable(t *testing.T, h *Harness, m *NftablesManager) {
	t.Helper()
	rules := shared.Rules{TCP: []shared.PortRule{{
		Port:        strconv.Itoa(selftestForwardedPort),
		Scope:       shared.ScopeForwarded,
		Description: "a published container port",
	}}}
	net := shared.NetworkSettings{Docker: shared.DockerConfig{
		Enabled:        true,
		CustomNetworks: []string{h.BridgeCIDR()},
		PublishedPorts: shared.PublishedPortsFiltered,
	}}
	if err := m.Apply(shared.RulesState{Current: rules}, shared.FirewallOptions{}, net); err != nil {
		t.Fatalf("applying the filtered table inside the peer: %v", err)
	}
}

// TestIntegration_AForwardedRuleOpensOnePortAndTheDenyClosesTheRest is the
// inbound half.
//
// Both ports are dialled before the table is applied as well as after. Without
// that control a silence on 110 would also be what an unwired leg, a peer that
// is not forwarding and a dead container all look like, and none of those is a
// finding about the rules — the same discipline the self-test's provers use.
func TestIntegration_AForwardedRuleOpensOnePortAndTheDenyClosesTheRest(t *testing.T) {
	h, m := forwardScopeHarness(t)

	for _, port := range []uint16{selftestForwardedPort, selftestUnforwardedPort} {
		crossed, err := crossesTo(h.ContainerAddr(), port)
		if err != nil {
			skipOrFailUnprovable(t, err.Error())
		}
		if !crossed {
			skipOrFailUnprovable(t, "with no table in the peer at all, port "+
				strconv.Itoa(int(port))+" on the container was not answered, so the leg is not "+
				"carrying packets and nothing measured below would mean anything")
		}
	}

	forwardScopeTable(t, h, m)

	crossed, err := crossesTo(h.ContainerAddr(), selftestForwardedPort)
	if err != nil {
		t.Fatalf("dialling the forwarded port: %v", err)
	}
	if !crossed {
		t.Errorf("port %d has a forwarded rule and is not reachable\n"+
			"the forwarded accepts are not in front of the deny, and every published port on a "+
			"host with this configuration is shut", selftestForwardedPort)
	}

	crossed, err = crossesTo(h.ContainerAddr(), selftestUnforwardedPort)
	if err != nil {
		t.Fatalf("dialling the unforwarded port: %v", err)
	}
	if crossed {
		t.Errorf("port %d has no forwarded rule and is reachable — the deny did nothing\n"+
			"that is the release's own defect: a deny behind the CIDR exceptions can never say "+
			"no, because an exception accepts any packet with either end inside the bridge range",
			selftestUnforwardedPort)
	}
}

// The container must keep its own way out. This is the failure that the
// acceptance window cannot see: SSH arrives on the input chain and stays up
// while every container on the host loses the network.
//
// The direction is what makes it sharp. The container's SYN has its source in
// the bridge range, so the deny — which needs the *destination* there — never
// matches it; the packet that matters is the SYN-ACK coming back, whose
// destination is the container and whose source is not. That is the deny's exact
// shape, and the only thing standing in front of it is the established accept at
// the top of the chain. Move that one call below addForwardPortRules and this
// test times out while the inbound test above stays green.
func TestIntegration_TheDenyDoesNotTouchOutboundFromTheBridge(t *testing.T) {
	h, m := forwardScopeHarness(t)

	ln, err := net.Listen("tcp", net.JoinHostPort(h.RouterAddr().String(),
		strconv.Itoa(forwardScopeListenPort)))
	if err != nil {
		skipOrFailUnprovable(t, "nothing could be bound on this side: "+err.Error())
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // ln.Close, from the cleanup above
			}
			_ = conn.Close()
		}
	}()

	open, err := h.DialFromContainer(h.RouterAddr(), forwardScopeListenPort, selftestProbeTimeout)
	if err != nil {
		skipOrFailUnprovable(t, "the container could not dial out at all: "+err.Error())
	}
	if !open {
		skipOrFailUnprovable(t, "the container could not reach the listener before any table "+
			"existed, so a failure with one applied would not be the firewall's doing")
	}

	forwardScopeTable(t, h, m)

	open, err = h.DialFromContainer(h.RouterAddr(), forwardScopeListenPort, selftestProbeTimeout)
	if err != nil {
		t.Fatalf("asking the container to dial out with the table applied: %v", err)
	}
	if !open {
		t.Error("a container's outbound traffic was dropped by the inbound deny\n" +
			"the reply to an outbound connection arrives with the bridge address as its " +
			"destination and a source outside it, which is what the deny matches. Nothing but " +
			"the established accept at the top of the forward chain keeps it there, and behind " +
			"that deny every container on the host loses the network at the next apply")
	}
}
