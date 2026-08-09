//go:build integration

package core

import (
	"encoding/json"
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
	if err := m.Apply(emptyState(), opts, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}

// applyEmptyIPv6 applies an empty rule set with IPv6 enabled.
func applyEmptyIPv6(t *testing.T, m *NftablesManager, ipv6 shared.IPv6Config) {
	t.Helper()
	if err := m.Apply(emptyState(), shared.FirewallOptions{}, ipv6, shared.DockerConfig{}); err != nil {
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
	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
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
	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
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
	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
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
	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
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
	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
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
	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
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

	// SYNFlood adds a rate-limit rule directly to the input chain (no separate chain).
	applyEmpty(t, m, shared.FirewallOptions{SYNFlood: true})
	count := ruleCount(t, m, "input")

	if count != base+1 {
		t.Errorf("SYNFlood: expected %d rules, got %d", base+1, count)
	}
}

func TestIntegration_Apply_ICMPFlood_AddsRule(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	// ICMPFlood adds a rate-limit rule directly to the input chain (no separate chain).
	applyEmpty(t, m, shared.FirewallOptions{ICMPFlood: true})
	count := ruleCount(t, m, "input")

	if count != base+1 {
		t.Errorf("ICMPFlood: expected %d rules, got %d", base+1, count)
	}
}

func TestIntegration_Apply_BogonFilter_AddsNineRules(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	applyEmpty(t, m, shared.FirewallOptions{Bogons: true})
	count := ruleCount(t, m, "input")

	// addBogonFilter defines exactly 9 bogon CIDRs.
	const bogonCount = 9
	if count != base+bogonCount {
		t.Errorf("Bogons: expected %d rules (base=%d + %d bogons), got %d",
			base+bogonCount, base, bogonCount, count)
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

	if err := m.Apply(state, shared.FirewallOptions{SSHBruteForce: true}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
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

	if err := m.Apply(state, shared.FirewallOptions{SSHBruteForce: true}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// sshbrute chain must have exactly 2 rules: rate-limit-accept + unconditional drop.
	count := ruleCount(t, m, "sshbrute")
	if count != 2 {
		t.Errorf("sshbrute chain: expected 2 rules (accept+drop), got %d", count)
	}
}

func TestIntegration_Apply_SSHBruteForce_DefaultsToPort22(t *testing.T) {
	m := newIntegrationManager(t)
	// No TCP rules — addSSHBruteForce falls back to port 22.
	if err := m.Apply(emptyState(), shared.FirewallOptions{SSHBruteForce: true}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if !hasChainName(t, m, "sshbrute") {
		t.Error("expected 'sshbrute' chain even without explicit SSH rule")
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

	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
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

	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
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

	if err := m.Apply(state, opts, shared.IPv6Config{Enabled: true}, shared.DockerConfig{}); err != nil {
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
		if err := m.Apply(state, opts, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
			t.Fatalf("Apply #%d: %v", i+1, err)
		}
	}

	countA := ruleCount(t, m, "input")
	if err := m.Apply(state, opts, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
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
	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
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
