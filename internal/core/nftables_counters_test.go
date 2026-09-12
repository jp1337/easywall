//go:build integration

package core

import (
	"os/exec"
	"strconv"
	"testing"

	"github.com/google/nftables"
	"github.com/jp1337/easywall/internal/shared"
)

// filteredDockerCustom is filteredDocker (nftables_forward_scope_test.go) with a
// concrete container network named directly, so addForwardPortRules renders
// without needing a real Docker bridge to autodetect. CustomNetworks reaches
// dockerCIDRs regardless of AllowBridgeNetworks — see Apply's assembly of it.
func filteredDockerCustom(cidr string) shared.DockerConfig {
	return shared.DockerConfig{
		Enabled:        true,
		PublishedPorts: shared.PublishedPortsFiltered,
		CustomNetworks: []string{cidr},
	}
}

// A rule that renders only into the forward chain must still have its counter
// read. Before this task RuleCounters walked the input chain alone, so a
// scope = "forwarded" rule's id never appeared in the map at all and every
// surface built on it — the ports page, the usage history — reported "never
// used" about a rule that may carry a container's entire mail service.
//
// This does not go through Apply. Apply's addPortAccept call for the input
// chain takes no notice of scope at all — shared.PortRule.FiltersHost exists,
// is unit-tested in internal/shared, and is never called from
// internal/core — so a scope = "forwarded" rule applied normally is *also*
// opened directly on the input chain today, which would make this test pass
// under the very mutation it exists to catch (narrowing RuleCounters back to
// the input chain would still find the id there). That is a real containment
// gap, in the opposite direction from this task's remit, and belongs to
// whichever task owns Apply's input-chain rendering — flagged in the report,
// not fixed here. So this builds a kernel table with a forward chain and
// nothing else, the same way NewNftablesManager's own reset()/Apply do
// internally, and writes the rule straight in with portAcceptRules — the
// builder addForwardPortRules itself uses. With no input chain in this
// table at all, "only in the forward chain" is not an assumption, it is the
// only chain that exists.
//
// No traffic is sent: the claim under test is that the id is *read*, the same
// claim TestIntegration_RuleCountersReportsTheReservedEstablishedID makes about
// the reserved id, and for the same reason — a zero-valued entry and no entry
// at all are two different failures, and this test is about the second one.
func TestIntegration_RuleCounters_ForwardOnlyRuleIsReported(t *testing.T) {
	m := newIntegrationManager(t)

	m.conn.AddTable(&nftables.Table{Name: tableName, Family: nftables.TableFamilyINet})
	if err := m.conn.Flush(); err != nil {
		t.Fatalf("create table: %v", err)
	}
	tbl := &nftables.Table{Name: tableName, Family: nftables.TableFamilyINet}
	fwd := m.conn.AddChain(&nftables.Chain{
		Name:     forwardChainName,
		Table:    tbl,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookForward,
		Priority: nftables.ChainPriorityRef(prioFilter),
		Policy:   policyDrop(),
	})
	if err := m.conn.Flush(); err != nil {
		t.Fatalf("create forward chain: %v", err)
	}

	rule := shared.PortRule{
		ID: "f0000000f001", Port: "12241", Description: "forwarded-only proof",
		Scope: shared.ScopeForwarded,
	}
	for _, r := range portAcceptRules(tbl, fwd, "tcp", rule) {
		m.conn.AddRule(r)
	}
	if err := m.conn.Flush(); err != nil {
		t.Fatalf("add the forward-chain rule: %v", err)
	}

	counters, err := m.RuleCounters()
	if err != nil {
		t.Fatalf("RuleCounters: %v", err)
	}
	if _, ok := counters[rule.ID]; !ok {
		t.Fatalf("RuleCounters has no entry for %q, a rule that renders only into the "+
			"forward chain; its kernel counter exists and nothing reads it, which reports "+
			"\"never used\" about a rule that may be carrying live traffic\ncounters: %v",
			rule.ID, counters)
	}
}

// A scope = "both" rule is one UI rule with two kernel rules, one per chain,
// and its counter has to be their sum — not whichever chain RuleCounters
// happens to read last. Distinguishing "sum" from "overwrite" needs two
// different, nonzero counts, so this sends real, separately-countable traffic
// down each chain: two connections addressed to this host (the input chain's
// copy) and three routed through it to the namespace on the other side (the
// forward chain's copy).
func TestIntegration_RuleCounters_BothScopeRuleSumsBothChains(t *testing.T) {
	for _, bin := range []string{"bash", "timeout"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("skipping: %s is not installed, and this test opens real TCP connections", bin)
		}
	}

	m := newIntegrationManager(t)
	r := newRouter(t)

	const port = 12242
	rule := shared.PortRule{
		ID: "b0000000b002", Port: strconv.Itoa(port), Description: "both-scope proof",
		Scope: shared.ScopeBoth,
	}
	rules := shared.Rules{TCP: []shared.PortRule{rule}}
	state := shared.RulesState{Current: rules, Staged: rules, Backup: rules}

	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{
		Docker: filteredDockerCustom(r.netB),
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Two connections addressed to the router itself: destined for this host,
	// so the input chain decides and only the input copy's counter can move.
	for i := 0; i < 2; i++ {
		tcpReaches(t, r.pidA, "10.77.1.1", port)
	}
	// Three connections addressed past the router, to the namespace on the
	// other side: routed, so the forward chain decides and only the forward
	// copy's counter can move. Neither destination runs a listener — the SYN
	// alone is what an accept rule counts, same as
	// TestIntegration_TheCounterMovesWhenAPortIsUsed relies on for the input
	// chain.
	for i := 0; i < 3; i++ {
		tcpReaches(t, r.pidA, r.addrB, port)
	}

	counters, err := m.RuleCounters()
	if err != nil {
		t.Fatalf("RuleCounters: %v", err)
	}
	got, ok := counters[rule.ID]
	if !ok {
		t.Fatalf("RuleCounters has no entry for the both-scope rule %q", rule.ID)
	}
	if got.Packets != 5 {
		t.Errorf("the both-scope rule counted %d packets for 2 connections to the host and "+
			"3 routed through it, want 5 (their sum). Anything else means RuleCounters read "+
			"one chain's kernel rule and discarded the other's, rather than adding them",
			got.Packets)
	}
}

// Real established traffic crossing the forward chain must not be counted
// under the reserved established id. addEstablishedAccept tags only the input
// chain's copy — TestEstablishedRuleIsTaggedAndCounted pins that at the
// construction level — and this is what proves it end to end, through
// RuleCounters itself, now that RuleCounters reads the forward chain too.
//
// routing.mode = "networks" naming the far namespace's subnet is enough to let
// a ping cross the forward chain without any port rule or Docker config: the
// exception at the foot of buildForwardChain accepts it unconditionally, and
// the reply is what conntrack classifies established — exactly what the
// established-accept rule matches, in both chains, but only one of them
// carries the id.
func TestIntegration_RuleCounters_TheForwardCopyOfEstablishedAcceptDoesNotContribute(t *testing.T) {
	m := newIntegrationManager(t)
	r := newRouter(t)

	if err := m.Apply(emptyState(), shared.FirewallOptions{}, shared.NetworkSettings{
		Routing: shared.RoutingConfig{Mode: shared.RoutingNetworks, Networks: []string{r.netB}},
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	before, err := m.RuleCounters()
	if err != nil {
		t.Fatalf("RuleCounters (before): %v", err)
	}
	beforePackets := before[ReservedIDEstablished].Packets

	if !r.reachable() {
		t.Skip("skipping: the router could not reach across itself, so nothing here would " +
			"generate the routed, established traffic this test depends on")
	}

	after, err := m.RuleCounters()
	if err != nil {
		t.Fatalf("RuleCounters (after): %v", err)
	}
	afterPackets := after[ReservedIDEstablished].Packets

	if afterPackets != beforePackets {
		t.Errorf("%s went from %d to %d packets after traffic that only ever crossed the "+
			"forward chain (nothing here was addressed to this host). The forward copy of "+
			"the established-accept rule is deliberately untagged; a changed figure here "+
			"means it is being summed under the input chain's id, which is exactly the "+
			"double count addEstablishedAccept's doc comment forbids",
			ReservedIDEstablished, beforePackets, afterPackets)
	}
}
