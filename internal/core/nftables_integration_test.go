//go:build integration

package core

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/nftables"
	"github.com/jp1337/easywall/internal/shared"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newIntegrationManager creates a real NftablesManager backed by the isolated
// network namespace created in TestMain. The test is skipped when nftables is
// unavailable (no CAP_NET_ADMIN). The easywall table is cleaned up on test
// completion so each test starts with a clean slate.
func newIntegrationManager(t *testing.T) *NftablesManager {
	t.Helper()
	m, err := NewNftablesManager()
	if err != nil {
		t.Skipf("skipping: nftables unavailable (%v) — run with CAP_NET_ADMIN", err)
	}
	t.Cleanup(func() {
		m.conn.DelTable(&nftables.Table{Name: tableName, Family: nftables.TableFamilyINet})
		_ = m.conn.Flush()
	})
	return m
}

// applyEmpty applies an empty rule set with the given options.
func applyEmpty(t *testing.T, m *NftablesManager, opts shared.FirewallOptions) {
	t.Helper()
	if err := m.Apply(emptyState(), opts, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}

// applyEmptyIPv6 applies an empty rule set with IPv6 enabled.
func applyEmptyIPv6(t *testing.T, m *NftablesManager, ipv6 shared.IPv6Config) {
	t.Helper()
	if err := m.Apply(emptyState(), shared.FirewallOptions{}, shared.NetworkSettings{IPv6: ipv6}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}

// easywallInetTable returns the easywall inet table descriptor.
func easywallInetTable() *nftables.Table {
	return &nftables.Table{Name: tableName, Family: nftables.TableFamilyINet}
}

// listChains returns all chains that belong to the easywall inet table.
func listChains(t *testing.T, m *NftablesManager) []*nftables.Chain {
	t.Helper()
	all, err := m.conn.ListChains()
	if err != nil {
		t.Fatalf("ListChains: %v", err)
	}
	var chains []*nftables.Chain
	for _, c := range all {
		if c.Table != nil && c.Table.Name == tableName {
			chains = append(chains, c)
		}
	}
	return chains
}

// hasChainName reports whether a chain with the given name exists.
func hasChainName(t *testing.T, m *NftablesManager, name string) bool {
	t.Helper()
	for _, c := range listChains(t, m) {
		if c.Name == name {
			return true
		}
	}
	return false
}

// getChain returns the chain with the given name, fatal if missing.
func getChain(t *testing.T, m *NftablesManager, name string) *nftables.Chain {
	t.Helper()
	for _, c := range listChains(t, m) {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("chain %q not found in table %q", name, tableName)
	return nil
}

// ruleCount returns the number of rules in the named chain.
func ruleCount(t *testing.T, m *NftablesManager, chainName string) int {
	t.Helper()
	chain := getChain(t, m, chainName)
	rules, err := m.conn.GetRules(easywallInetTable(), chain)
	if err != nil {
		t.Fatalf("GetRules(%s): %v", chainName, err)
	}
	return len(rules)
}

// inputChainText returns the input chain exactly as `nft list` prints it, one
// rule per line in kernel order.
//
// Rule counts cannot express order, and order is the whole question for
// anything that meters traffic before accepting it. This is also what an
// operator sees when they check the box themselves.
func inputChainText(t *testing.T, _ *NftablesManager) []string {
	t.Helper()
	return chainText(t, "input")
}

// chainText does the same for any chain in the easywall table.
func chainText(t *testing.T, name string) []string {
	t.Helper()
	out, err := exec.Command("nft", "list", "chain", "inet", tableName, name).CombinedOutput()
	if err != nil {
		t.Fatalf("nft list chain %s: %v\n%s", name, err, out)
	}
	var rules []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "table ") || strings.HasPrefix(line, "chain ") ||
			strings.HasPrefix(line, "type ") || line == "}" {
			continue
		}
		rules = append(rules, line)
	}
	return rules
}

// indexOfRule returns the position of the first rule containing every fragment,
// or -1.
func indexOfRule(rules []string, fragments ...string) int {
	for i, r := range rules {
		all := true
		for _, f := range fragments {
			if !strings.Contains(r, f) {
				all = false
				break
			}
		}
		if all {
			return i
		}
	}
	return -1
}

// baseInputRules returns the number of rules in the input chain of an empty
// (no options, no ports) apply. Used to compute expected deltas.
func baseInputRules(t *testing.T, m *NftablesManager) int {
	t.Helper()
	applyEmpty(t, m, shared.FirewallOptions{})
	return ruleCount(t, m, "input")
}

// ---------------------------------------------------------------------------
// NewNftablesManager
// ---------------------------------------------------------------------------

func TestIntegration_NewNftablesManager(t *testing.T) {
	m := newIntegrationManager(t)
	if m == nil || m.conn == nil {
		t.Fatal("expected valid manager with non-nil conn")
	}
}

// ---------------------------------------------------------------------------
// Reset
// ---------------------------------------------------------------------------

func TestIntegration_Reset_CreatesTable(t *testing.T) {
	m := newIntegrationManager(t)

	if err := m.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	tables, err := m.conn.ListTables()
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}
	found := false
	for _, tbl := range tables {
		if tbl.Name == tableName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected table %q to exist after Reset", tableName)
	}
}

func TestIntegration_Reset_Idempotent(t *testing.T) {
	m := newIntegrationManager(t)
	for i := 0; i < 3; i++ {
		if err := m.Reset(); err != nil {
			t.Fatalf("Reset call %d: %v", i+1, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Apply — base chains and policies
// ---------------------------------------------------------------------------

func TestIntegration_Apply_CreatesBaseChains(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{})

	for _, name := range []string{"input", "output", "forward"} {
		if !hasChainName(t, m, name) {
			t.Errorf("expected chain %q to exist after Apply", name)
		}
	}
}

func TestIntegration_Apply_InputPolicy_Drop(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{})

	chain := getChain(t, m, "input")
	if chain.Policy == nil {
		t.Fatal("input chain: Policy is nil")
	}
	if *chain.Policy != nftables.ChainPolicyDrop {
		t.Errorf("input chain policy = %v, want DROP", *chain.Policy)
	}
}

func TestIntegration_Apply_OutputPolicy_Accept(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{})

	chain := getChain(t, m, "output")
	if chain.Policy == nil {
		t.Fatal("output chain: Policy is nil")
	}
	if *chain.Policy != nftables.ChainPolicyAccept {
		t.Errorf("output chain policy = %v, want ACCEPT", *chain.Policy)
	}
}

func TestIntegration_Apply_ForwardPolicy_Drop(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{})

	chain := getChain(t, m, "forward")
	if chain.Policy == nil {
		t.Fatal("forward chain: Policy is nil")
	}
	if *chain.Policy != nftables.ChainPolicyDrop {
		t.Errorf("forward chain policy = %v, want DROP", *chain.Policy)
	}
}

// ---------------------------------------------------------------------------
// Apply — base INPUT rules (loopback, established, ICMPv4)
// ---------------------------------------------------------------------------

func TestIntegration_Apply_BaseRules_Present(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{})

	// Minimum: loopback(1) + established(1) + ICMPv4 types {0,3,11,12}(4) = 6
	count := ruleCount(t, m, "input")
	if count < 6 {
		t.Errorf("expected at least 6 base rules in input chain, got %d", count)
	}
}

// The ICMPv6 exemptions belong to filter mode. Under block the traffic is gone
// before they would be reached; comparing against block is what isolates them.
func TestIntegration_Apply_ICMPv6_AddsRules_UnderFilterMode(t *testing.T) {
	m := newIntegrationManager(t)

	applyEmptyIPv6(t, m, shared.IPv6Config{Mode: shared.IPv6Block})
	blocked := ruleCount(t, m, "input")

	// 6 base ICMPv6 types (1,2,3,4,128,129), and block contributes one
	// family-wide drop rule of its own.
	applyEmptyIPv6(t, m, shared.IPv6Config{Mode: shared.IPv6Filter})
	filtered := ruleCount(t, m, "input")

	if filtered != blocked+5 {
		t.Errorf("expected 6 ICMPv6 rules in place of block's single drop, got %d "+
			"(filter=%d, block=%d)", filtered-blocked, filtered, blocked)
	}
}

func TestIntegration_Apply_ICMPv6_RouterAdvertisement(t *testing.T) {
	m := newIntegrationManager(t)

	applyEmptyIPv6(t, m, shared.IPv6Config{Mode: shared.IPv6Filter})
	base := ruleCount(t, m, "input")

	applyEmptyIPv6(t, m, shared.IPv6Config{Mode: shared.IPv6Filter, ICMPAllowRouterAdvertisement: true})
	withRA := ruleCount(t, m, "input")

	// Types 133+134 = 2 extra rules
	if withRA != base+2 {
		t.Errorf("expected 2 extra RA rules, got %d (base=%d, withRA=%d)", withRA-base, base, withRA)
	}
}

func TestIntegration_Apply_ICMPv6_NeighborAdvertisement(t *testing.T) {
	m := newIntegrationManager(t)

	applyEmptyIPv6(t, m, shared.IPv6Config{Mode: shared.IPv6Filter})
	base := ruleCount(t, m, "input")

	applyEmptyIPv6(t, m, shared.IPv6Config{Mode: shared.IPv6Filter, ICMPAllowNeighborAdvertisement: true})
	withNA := ruleCount(t, m, "input")

	// Types 135+136 = 2 extra rules
	if withNA != base+2 {
		t.Errorf("expected 2 extra NA rules, got %d (base=%d, withNA=%d)", withNA-base, base, withNA)
	}
}

// ---------------------------------------------------------------------------
// Apply — ports
// ---------------------------------------------------------------------------

func TestIntegration_Apply_TCPPort_AddsRule(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	state := emptyState()
	state.Current.TCP = []shared.PortRule{{Port: "80", Description: "HTTP"}}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply with TCP port: %v", err)
	}

	count := ruleCount(t, m, "input")
	if count != base+1 {
		t.Errorf("expected %d rules (base+1 for port 80), got %d", base+1, count)
	}
}

func TestIntegration_Apply_UDPPort_AddsRule(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	state := emptyState()
	state.Current.UDP = []shared.PortRule{{Port: "53", Description: "DNS"}}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply with UDP port: %v", err)
	}

	count := ruleCount(t, m, "input")
	if count != base+1 {
		t.Errorf("expected %d rules (base+1 for UDP/53), got %d", base+1, count)
	}
}

func TestIntegration_Apply_PortRange_AddsRule(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	state := emptyState()
	state.Current.TCP = []shared.PortRule{{Port: "8000:9000"}}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply with port range: %v", err)
	}

	count := ruleCount(t, m, "input")
	if count != base+1 {
		t.Errorf("expected %d rules (base+1 for range 8000:9000), got %d", base+1, count)
	}
}

func TestIntegration_Apply_MultiplePortsAndProtocols(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	state := emptyState()
	state.Current.TCP = []shared.PortRule{{Port: "80"}, {Port: "443"}, {Port: "8080"}}
	state.Current.UDP = []shared.PortRule{{Port: "53"}, {Port: "123"}}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	count := ruleCount(t, m, "input")
	if count != base+5 {
		t.Errorf("expected %d rules (base + 3 TCP + 2 UDP), got %d", base+5, count)
	}
}

// ---------------------------------------------------------------------------
// Apply — blacklist / whitelist
// ---------------------------------------------------------------------------

func TestIntegration_Apply_Blacklist_AddsRules(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	state := emptyState()
	state.Current.Blacklist = []string{"192.0.2.1", "198.51.100.0/24", "2001:db8::1"}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply with blacklist: %v", err)
	}

	count := ruleCount(t, m, "input")
	if count != base+3 {
		t.Errorf("expected %d rules (base + 3 blacklist entries), got %d", base+3, count)
	}
}

func TestIntegration_Apply_Whitelist_AddsRules(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	state := emptyState()
	state.Current.Whitelist = []string{"10.0.0.0/8", "172.16.0.0/12"}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply with whitelist: %v", err)
	}

	count := ruleCount(t, m, "input")
	if count != base+2 {
		t.Errorf("expected %d rules (base + 2 whitelist entries), got %d", base+2, count)
	}
}

// ---------------------------------------------------------------------------
// Apply — protection options
// ---------------------------------------------------------------------------

func TestIntegration_Apply_InvalidPackets_AddsRule(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	applyEmpty(t, m, shared.FirewallOptions{InvalidPackets: true})
	count := ruleCount(t, m, "input")

	if count != base+1 {
		t.Errorf("InvalidPackets: expected %d rules, got %d", base+1, count)
	}
}

func TestIntegration_Apply_Fragments_AddsRule(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	applyEmpty(t, m, shared.FirewallOptions{Fragments: true})
	count := ruleCount(t, m, "input")

	if count != base+1 {
		t.Errorf("Fragments: expected %d rules, got %d", base+1, count)
	}
}

func TestIntegration_Apply_BroadcastDrop_AddsRule(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	applyEmpty(t, m, shared.FirewallOptions{DropBroadcast: true})
	count := ruleCount(t, m, "input")

	if count != base+1 {
		t.Errorf("DropBroadcast: expected %d rules, got %d", base+1, count)
	}
}

func TestIntegration_Apply_MulticastDrop_AddsRule(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	applyEmpty(t, m, shared.FirewallOptions{DropMulticast: true})
	count := ruleCount(t, m, "input")

	if count != base+1 {
		t.Errorf("DropMulticast: expected %d rules, got %d", base+1, count)
	}
}

func TestIntegration_Apply_SYNFlood_AddsRule(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	// Two rules, one per address family: the rate is enforced per source
	// address, and the source address is a different width in each. A single
	// rule here was the shared counter that let one host starve the rest.
	applyEmpty(t, m, shared.FirewallOptions{SYNFlood: true})
	count := ruleCount(t, m, "input")

	if count != base+2 {
		t.Errorf("SYNFlood: expected %d rules (one per family), got %d", base+2, count)
	}
}

func TestIntegration_Apply_ICMPFlood_AddsRule(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	// One rule per family — and the IPv6 one is not a formality: it matches
	// ICMPv6 type 128, which the IPv4-only rule this replaced never did, so an
	// echo flood over IPv6 went straight past the module.
	applyEmpty(t, m, shared.FirewallOptions{ICMPFlood: true})
	count := ruleCount(t, m, "input")

	if count != base+2 {
		t.Errorf("ICMPFlood: expected %d rules (one per family), got %d", base+2, count)
	}
}

func TestIntegration_Apply_BogonFilter_AddsOneRulePerRange(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	applyEmpty(t, m, shared.FirewallOptions{Bogons: true})

	// The drops live in their own chain, so an address the operator allowed can
	// `return` past them — a return in the base chain would fall through to the
	// drop policy instead. The input chain therefore gains exactly one rule, the
	// jump that carries the family and interface tests.
	if count := ruleCount(t, m, "input"); count != base+1 {
		t.Errorf("Bogons: expected %d input rules (base=%d + the jump), got %d",
			base+1, base, count)
	}

	// Eleven ranges — the same eleven filters.md lists. It used to list two
	// that were not here and omit two that were.
	const bogonCount = 11
	if count := ruleCount(t, m, "bogon"); count != bogonCount {
		t.Errorf("Bogons: expected %d drops in the bogon chain, got %d", bogonCount, count)
	}
}

// ---------------------------------------------------------------------------
// Apply — named chains (portscan, sshbrute)
// ---------------------------------------------------------------------------

func TestIntegration_Apply_PortScan_CreatesChain(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{PortScan: true})

	if !hasChainName(t, m, "portscan") {
		t.Error("expected 'portscan' chain when PortScan=true")
	}
}

func TestIntegration_Apply_PortScan_AddsSevenScanRules(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	applyEmpty(t, m, shared.FirewallOptions{PortScan: true})
	count := ruleCount(t, m, "input")

	// addPortScanPrevention adds 7 scan-pattern rules to the input chain.
	const scanRules = 7
	if count != base+scanRules {
		t.Errorf("PortScan: expected %d rules (base=%d + %d scan rules), got %d",
			base+scanRules, base, scanRules, count)
	}
}

func TestIntegration_Apply_SSHBruteForce_CreatesChain(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()
	state.Current.TCP = []shared.PortRule{{Port: "22", SSH: true}}

	if err := m.Apply(state, shared.FirewallOptions{SSHBruteForce: true}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if !hasChainName(t, m, "sshbrute") {
		t.Error("expected 'sshbrute' chain when SSHBruteForce=true")
	}
}

func TestIntegration_Apply_SSHBruteForce_ChainHasRateLimitAndDrop(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()
	state.Current.TCP = []shared.PortRule{{Port: "22", SSH: true}}

	if err := m.Apply(state, shared.FirewallOptions{SSHBruteForce: true}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Three rules: a per-source meter for each family, then accept for anyone
	// still within their own rate. The drop moved to sshbrute-over, which is
	// where the log belongs too — the meter must be evaluated once per packet,
	// and a log rule repeating the match would consume a second token.
	count := ruleCount(t, m, "sshbrute")
	if count != 3 {
		t.Errorf("sshbrute chain: expected 3 rules (two meters + accept), got %d", count)
	}
	if !hasChainName(t, m, "sshbrute-over") {
		t.Error("expected an 'sshbrute-over' chain holding the drop")
	}
	if over := ruleCount(t, m, "sshbrute-over"); over != 1 {
		t.Errorf("sshbrute-over: expected just the drop, got %d rules", over)
	}
}

func TestIntegration_Apply_SSHBruteForce_DefaultsToPort22(t *testing.T) {
	m := newIntegrationManager(t)
	// No TCP rules — addSSHBruteForce falls back to port 22.
	if err := m.Apply(emptyState(), shared.FirewallOptions{SSHBruteForce: true}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if !hasChainName(t, m, "sshbrute") {
		t.Error("expected 'sshbrute' chain even without explicit SSH rule")
	}
}

// A port marked as SSH used to jump to the sshbrute chain, which exists only
// while the module is switched on. Turning the module off therefore produced a
// rule pointing at a chain that was not there: the apply failed, the rollback
// failed identically, and the kernel was left holding a table with no chains
// and no policy — the host completely unfiltered, from one checkbox.
//
// The first-run wizard marks the SSH port for every new installation, so this
// is now on the path everybody takes.
func TestIntegration_Apply_SSHPortAppliesWithBruteForceOff(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()
	state.Current.TCP = []shared.PortRule{{Port: "22", Description: "SSH", SSH: true}}

	if err := m.Apply(state, shared.FirewallOptions{SSHBruteForce: false}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply with SSH port and brute force disabled: %v", err)
	}

	rules := inputChainText(t, m)
	if indexOfRule(rules, "tcp dport 22", "accept") < 0 {
		t.Errorf("expected port 22 to be accepted, input chain holds:\n%s", strings.Join(rules, "\n"))
	}
	if i := indexOfRule(rules, "sshbrute"); i >= 0 {
		t.Errorf("rule %d references the sshbrute chain although the module is off: %s", i, rules[i])
	}
	if !m.Enforcing() {
		t.Error("firewall reports itself not enforcing after a successful apply")
	}
}

// With the module on, a new connection must meet the meter before it meets the
// accept — otherwise the protection is present in the ruleset and bypassed by
// every packet.
func TestIntegration_Apply_SSHBruteForce_MetersBeforeAccepting(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()
	state.Current.TCP = []shared.PortRule{{Port: "22", Description: "SSH", SSH: true}}

	if err := m.Apply(state, shared.FirewallOptions{SSHBruteForce: true}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	rules := inputChainText(t, m)
	meter := indexOfRule(rules, "tcp dport 22", "jump sshbrute")
	accept := indexOfRule(rules, "tcp dport 22", "accept")
	switch {
	case meter < 0:
		t.Fatalf("no metering rule for the SSH port:\n%s", strings.Join(rules, "\n"))
	case accept < 0:
		t.Fatalf("port 22 is never accepted:\n%s", strings.Join(rules, "\n"))
	case meter > accept:
		t.Errorf("the accept at %d precedes the meter at %d — brute force protection never sees a packet:\n%s",
			accept, meter, strings.Join(rules, "\n"))
	}
}

// A range marked as SSH was parsed with parsePort, which returns 0 for anything
// containing a colon, and the port was skipped. The module reported itself
// enabled and metered nothing at all.
func TestIntegration_Apply_SSHBruteForce_MetersAPortRange(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()
	state.Current.TCP = []shared.PortRule{{Port: "2200:2210", Description: "SSH", SSH: true}}

	if err := m.Apply(state, shared.FirewallOptions{SSHBruteForce: true}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	rules := inputChainText(t, m)
	if indexOfRule(rules, "2200-2210", "jump sshbrute") < 0 {
		t.Errorf("port range marked as SSH is not metered:\n%s", strings.Join(rules, "\n"))
	}
}

// A Docker network with padding survives shared.ValidateNetworkList (which
// trims before parsing) and addForwardExceptions' cidrMatch (which also
// trims), so it reaches the kernel with a forward exception — but
// addCIDRAccept did not trim, so the same list got no input accept. Half of
// what the operator asked for, with nothing said. Reachable only through a
// hand-edited easywall.toml and SIGHUP, since parseIPList trims what arrives
// from the web interface.
func TestIntegration_Apply_DockerNetworkAcceptsPaddedCIDR(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()

	docker := shared.DockerConfig{Enabled: true, CustomNetworks: []string{"  10.8.0.0/24  "}}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{Docker: docker}); err != nil {
		t.Fatalf("Apply with a padded docker network: %v", err)
	}

	rules := inputChainText(t, m)
	if indexOfRule(rules, "10.8.0.0/24", "accept") < 0 {
		t.Errorf("padded docker network %q got no input accept; input chain holds:\n%s",
			docker.CustomNetworks[0], strings.Join(rules, "\n"))
	}
}

// ---------------------------------------------------------------------------
// Apply — port forwarding
// ---------------------------------------------------------------------------

func TestIntegration_Apply_Forwarding_CreatesPreRoutingChain(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()
	state.Current.Forwarding = []shared.ForwardingRule{
		{Protocol: "tcp", SourcePort: 8080, DestPort: 80},
	}

	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply with forwarding: %v", err)
	}

	if !hasChainName(t, m, "prerouting") {
		t.Error("expected 'prerouting' NAT chain when forwarding rules are present")
	}
}

func TestIntegration_Apply_Forwarding_CorrectRuleCount(t *testing.T) {
	m := newIntegrationManager(t)
	state := emptyState()
	state.Current.Forwarding = []shared.ForwardingRule{
		{Protocol: "tcp", SourcePort: 8080, DestPort: 80},
		{Protocol: "udp", SourcePort: 5353, DestPort: 53},
	}

	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	count := ruleCount(t, m, "prerouting")
	if count != 2 {
		t.Errorf("expected 2 forwarding rules in prerouting, got %d", count)
	}
}

func TestIntegration_Apply_NoForwarding_NoPreRoutingChain(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{})

	if hasChainName(t, m, "prerouting") {
		t.Error("prerouting chain should NOT exist when there are no forwarding rules")
	}
}

// ---------------------------------------------------------------------------
// Apply — combined / idempotency
// ---------------------------------------------------------------------------

func TestIntegration_Apply_AllOptions(t *testing.T) {
	m := newIntegrationManager(t)

	opts := shared.FirewallOptions{
		SSHBruteForce:  true,
		ICMPFlood:      true,
		SYNFlood:       true,
		PortScan:       true,
		InvalidPackets: true,
		Fragments:      true,
		Bogons:         true,
		DropBroadcast:  true,
		DropMulticast:  true,
	}
	state := emptyState()
	state.Current.TCP = []shared.PortRule{
		{Port: "22", SSH: true},
		{Port: "80"},
		{Port: "443"},
	}
	state.Current.UDP = []shared.PortRule{{Port: "53"}}
	state.Current.Blacklist = []string{"203.0.113.0/24"}
	state.Current.Whitelist = []string{"10.0.0.0/8"}
	state.Current.Forwarding = []shared.ForwardingRule{
		{Protocol: "tcp", SourcePort: 8080, DestPort: 80},
	}

	if err := m.Apply(state, opts, shared.NetworkSettings{IPv6: shared.IPv6Config{Enabled: true}}); err != nil {
		t.Fatalf("Apply all options: %v", err)
	}

	for _, chain := range []string{"input", "output", "forward", "sshbrute", "portscan", "prerouting"} {
		if !hasChainName(t, m, chain) {
			t.Errorf("expected chain %q to exist", chain)
		}
	}
}

func TestIntegration_Apply_Idempotent(t *testing.T) {
	m := newIntegrationManager(t)

	state := emptyState()
	state.Current.TCP = []shared.PortRule{{Port: "443"}}
	opts := shared.FirewallOptions{InvalidPackets: true, Fragments: true}

	for i := 0; i < 3; i++ {
		if err := m.Apply(state, opts, shared.NetworkSettings{}); err != nil {
			t.Fatalf("Apply #%d: %v", i+1, err)
		}
	}

	countA := ruleCount(t, m, "input")
	if err := m.Apply(state, opts, shared.NetworkSettings{}); err != nil {
		t.Fatal(err)
	}
	countB := ruleCount(t, m, "input")

	if countA != countB {
		t.Errorf("Apply is not idempotent: %d rules → %d rules on 4th call", countA, countB)
	}
}

// ---------------------------------------------------------------------------
// Snapshot
// ---------------------------------------------------------------------------

func TestIntegration_Snapshot_ReturnsValidJSON(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{})

	snap, err := m.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap) == 0 {
		t.Fatal("Snapshot returned empty data")
	}

	var data map[string]interface{}
	if err := json.Unmarshal(snap, &data); err != nil {
		t.Fatalf("Snapshot is not valid JSON: %v — raw: %s", err, snap)
	}
	if _, ok := data["timestamp"]; !ok {
		t.Error("snapshot missing 'timestamp' field")
	}
	if _, ok := data["tables"]; !ok {
		t.Error("snapshot missing 'tables' field")
	}
}

func TestIntegration_Snapshot_TablesCountReflectsState(t *testing.T) {
	m := newIntegrationManager(t)

	applyEmpty(t, m, shared.FirewallOptions{})

	snap, err := m.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot after Apply: %v", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(snap, &data); err != nil {
		t.Fatalf("snapshot is not valid JSON: %v", err)
	}

	// tables is now a JSON array, not a count.
	tables, ok := data["tables"].([]interface{})
	if !ok {
		t.Fatalf("expected 'tables' to be an array, got %T", data["tables"])
	}
	if len(tables) == 0 {
		t.Error("expected at least 1 table entry after Apply")
	}
}

func TestIntegration_Snapshot_ContainsEasywallTable(t *testing.T) {
	m := newIntegrationManager(t)
	applyEmpty(t, m, shared.FirewallOptions{})

	snap, err := m.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(snap, &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	tables, _ := data["tables"].([]interface{})
	var easywallEntry map[string]interface{}
	for _, raw := range tables {
		entry, _ := raw.(map[string]interface{})
		if entry["name"] == tableName {
			easywallEntry = entry
			break
		}
	}
	if easywallEntry == nil {
		t.Fatalf("easywall table not found in snapshot; tables: %v", tables)
	}
	if easywallEntry["family"] != "inet" {
		t.Errorf("expected family 'inet', got %v", easywallEntry["family"])
	}
}

func TestIntegration_Snapshot_ChainsPopulated(t *testing.T) {
	m := newIntegrationManager(t)
	// Apply with a known set so we have deterministic chains.
	applyEmpty(t, m, shared.FirewallOptions{})

	snap, err := m.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	var data map[string]interface{}
	_ = json.Unmarshal(snap, &data)

	tables, _ := data["tables"].([]interface{})
	for _, raw := range tables {
		entry, _ := raw.(map[string]interface{})
		if entry["name"] == tableName {
			chains, _ := entry["chains"].([]interface{})
			// A minimal Apply creates input, output, forward.
			if len(chains) < 3 {
				t.Errorf("expected at least 3 chains in easywall table, got %d", len(chains))
			}
			// Verify each chain entry has a name field.
			for _, c := range chains {
				chain, _ := c.(map[string]interface{})
				if chain["name"] == "" {
					t.Error("chain entry missing 'name' field")
				}
			}
			return
		}
	}
	t.Fatal("easywall table not found in snapshot")
}

func TestIntegration_Snapshot_RuleCountsPresent(t *testing.T) {
	m := newIntegrationManager(t)
	// Apply with some rules so the input chain has a non-trivial rule count.
	state := emptyState()
	state.Current.TCP = []shared.PortRule{{Port: "80"}, {Port: "443"}}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	snap, err := m.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	var data map[string]interface{}
	_ = json.Unmarshal(snap, &data)

	tables, _ := data["tables"].([]interface{})
	for _, raw := range tables {
		entry, _ := raw.(map[string]interface{})
		if entry["name"] != tableName {
			continue
		}
		chains, _ := entry["chains"].([]interface{})
		for _, c := range chains {
			chain, _ := c.(map[string]interface{})
			if chain["name"] == "input" {
				// rules count must be a number and > 0 (loopback+established+ICMP+2 ports)
				rules, ok := chain["rules"].(float64)
				if !ok {
					t.Fatalf("rules field is not a number: %T", chain["rules"])
				}
				if rules < 1 {
					t.Errorf("input chain should have rules, got %v", rules)
				}
				return
			}
		}
	}
	t.Fatal("input chain not found in snapshot")
}

// A source restriction is one rule per source, and no source is still one rule.
func TestIntegration_Apply_PortSources(t *testing.T) {
	m := newIntegrationManager(t)

	base := baseInputRules(t, m)

	rules := shared.Rules{TCP: []shared.PortRule{
		{Port: "443", Description: "HTTPS"},                                 // 1 rule
		{Port: "8123", Sources: []string{"10.0.0.0/8", "192.168.0.0/16"}},   // 2 rules
		{Port: "9090", Sources: []string{"# only the LAN", "", "10.1.2.3"}}, // 1 rule
	}}
	state := shared.RulesState{Current: rules, Staged: rules, Backup: rules}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if got, want := ruleCount(t, m, "input"), base+4; got != want {
		t.Errorf("expected %d rules (base + 1 + 2 + 1), got %d\ninput chain:\n  %s",
			want, got, strings.Join(inputChainText(t, m), "\n  "))
	}

	text := inputChainText(t, m)
	// The source match precedes the port test in the rule nft prints, and an
	// unrestricted port carries no address match at all.
	if i := indexOfRule(text, "ip saddr 10.0.0.0/8", "dport 8123"); i < 0 {
		t.Errorf("no rule restricts 8123 to 10.0.0.0/8\ninput chain:\n  %s",
			strings.Join(text, "\n  "))
	}
	if i := indexOfRule(text, "ip saddr 192.168.0.0/16", "dport 8123"); i < 0 {
		t.Errorf("no rule restricts 8123 to 192.168.0.0/16\ninput chain:\n  %s",
			strings.Join(text, "\n  "))
	}
	if i := indexOfRule(text, "dport 443"); i < 0 {
		t.Errorf("443 is not open\ninput chain:\n  %s", strings.Join(text, "\n  "))
	}
	if i := indexOfRule(text, "saddr", "dport 443"); i >= 0 {
		t.Errorf("443 has no sources and must carry no address match, got %q", text[i])
	}
	// The comment/blank entries in 9090's Sources must be skipped, not turned
	// into "anywhere": there must be a rule binding it to 10.1.2.3, and no rule
	// may open dport 9090 without a saddr match.
	if i := indexOfRule(text, "ip saddr 10.1.2.3", "dport 9090"); i < 0 {
		t.Errorf("no rule restricts 9090 to 10.1.2.3\ninput chain:\n  %s",
			strings.Join(text, "\n  "))
	}
	// indexOfRule returns the FIRST match, which here is always the restricted
	// rule above (it also contains "dport 9090"), so it could never see an
	// unrestricted sibling rule. Search every match instead.
	for _, line := range text {
		if strings.Contains(line, "dport 9090") && !strings.Contains(line, "saddr") {
			t.Errorf("9090 has sources and must not carry an unrestricted rule, got %q", line)
		}
	}
}

// A source list with nothing usable in it (all comments, all blank) must open
// no rule at all — not the world. This is distinct from the "no Sources"
// case above: a present-but-empty-of-substance list is an operator who has
// not finished typing, and the one wrong answer available is "anywhere".
func TestIntegration_Apply_PortSources_AllCommentsOpensNothing(t *testing.T) {
	m := newIntegrationManager(t)

	base := baseInputRules(t, m)

	rules := shared.Rules{TCP: []shared.PortRule{
		{Port: "7000", Sources: []string{"# nothing usable here", ""}},
	}}
	state := shared.RulesState{Current: rules, Staged: rules, Backup: rules}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if got, want := ruleCount(t, m, "input"), base; got != want {
		t.Errorf("expected %d rules (all-comment source list opens nothing), got %d\ninput chain:\n  %s",
			want, got, strings.Join(inputChainText(t, m), "\n  "))
	}
	if i := indexOfRule(inputChainText(t, m), "dport 7000"); i >= 0 {
		t.Errorf("7000 must not be open when its source list has nothing usable, got %q", inputChainText(t, m)[i])
	}
}
