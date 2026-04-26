//go:build integration

package core

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/google/nftables"
	"github.com/jp1337/easywall/internal/shared"
)

// ---------------------------------------------------------------------------
// NewFirewall
// ---------------------------------------------------------------------------

func TestIntegration_NewFirewall(t *testing.T) {
	// Confirm nftables is available before attempting NewFirewall.
	m := newIntegrationManager(t)
	defer func() {
		m.conn.DelTable(&nftables.Table{Name: tableName, Family: nftables.TableFamilyINet})
		_ = m.conn.Flush()
	}()

	cfg := newTestConfig(t)
	fw, err := NewFirewall(cfg)
	if err != nil {
		t.Fatalf("NewFirewall: %v", err)
	}
	if fw == nil {
		t.Fatal("expected non-nil Firewall")
	}
	// Clean up the table created by NewFirewall's internal NftablesManager.
	defer func() {
		fw.nft.conn.DelTable(&nftables.Table{Name: tableName, Family: nftables.TableFamilyINet})
		_ = fw.nft.conn.Flush()
	}()
}

// ---------------------------------------------------------------------------
// Firewall.Apply — full lifecycle with real nftables connection
// ---------------------------------------------------------------------------

// newTestFirewallWithRealNft creates a Firewall backed by a real NftablesManager
// so that Apply can actually flush rules to the (isolated) kernel.
func newTestFirewallWithRealNft(t *testing.T) *Firewall {
	t.Helper()
	m := newIntegrationManager(t)
	cfg := newTestConfig(t)

	store, err := NewRulesStore(cfg.RulesPath())
	if err != nil {
		t.Fatalf("NewRulesStore: %v", err)
	}

	return &Firewall{
		cfg:        cfg,
		nft:        m,
		rules:      store,
		acceptance: NewAcceptance(cfg.AcceptanceDuration()),
	}
}

func TestIntegration_Firewall_Apply_Accepted(t *testing.T) {
	fw := newTestFirewallWithRealNft(t)

	_ = fw.rules.SaveStaged("tcp", []shared.PortRule{{Port: "443", Description: "HTTPS"}})

	// Accept the new rules 200 ms after Apply starts waiting.
	go func() {
		time.Sleep(200 * time.Millisecond)
		fw.Accept()
	}()

	if err := fw.Apply("integration-test"); err != nil {
		t.Fatalf("Firewall.Apply: %v", err)
	}

	// After acceptance, LastApply must be set.
	status := fw.Status()
	if status.LastApply == "" {
		t.Error("expected LastApply to be set after a successful accepted Apply")
	}
	if status.Acceptance != shared.AcceptanceIdle {
		t.Errorf("expected AcceptanceIdle after completion, got %s", status.Acceptance)
	}
}

func TestIntegration_Firewall_Apply_Timeout_Rollback(t *testing.T) {
	fw := newTestFirewallWithRealNft(t)
	// Very short acceptance window so the test doesn't block long.
	fw.cfg.Acceptance.Duration = 1 // 1 second
	fw.acceptance = NewAcceptance(fw.cfg.AcceptanceDuration())

	_ = fw.rules.SaveStaged("tcp", []shared.PortRule{{Port: "80"}})

	// Do NOT call Accept — the window should time out and roll back.
	err := fw.Apply("integration-test")
	if err != nil {
		t.Fatalf("Apply should return nil on timeout rollback, got: %v", err)
	}

	// After rollback the staged and current rules should reflect the backup state.
	state, err := fw.rules.GetState()
	if err != nil {
		t.Fatal(err)
	}
	// Current should have been rolled back to the (empty) backup.
	if len(state.Current.TCP) != 0 {
		t.Errorf("expected empty current rules after rollback, got %+v", state.Current.TCP)
	}
}

func TestIntegration_Firewall_Apply_SnapshotSaved(t *testing.T) {
	fw := newTestFirewallWithRealNft(t)

	go func() {
		time.Sleep(100 * time.Millisecond)
		fw.Accept()
	}()

	if err := fw.Apply("integration-test"); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// A snapshot file should have been written to LogDir.
	entries, err := listDir(t, fw.cfg.LogDir)
	if err != nil {
		t.Fatalf("list LogDir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least one snapshot file in LogDir after Apply")
	}
}

func listDir(t *testing.T, dir string) ([]string, error) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

// ---------------------------------------------------------------------------
// addFinalLog — requires LogBlocked = true
// ---------------------------------------------------------------------------

func TestIntegration_Apply_FinalLog_AddsRule(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	opts := shared.FirewallOptions{
		LogBlocked:      true,
		LogBlockedLimit: 30,
	}
	applyEmpty(t, m, opts)
	count := ruleCount(t, m, "input")

	if count != base+1 {
		t.Errorf("LogBlocked: expected %d rules (base+1), got %d", base+1, count)
	}
}

func TestIntegration_Apply_FinalLog_DefaultLimit(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	// LogBlockedLimit = 0 should fall back to default (60).
	applyEmpty(t, m, shared.FirewallOptions{LogBlocked: true, LogBlockedLimit: 0})
	count := ruleCount(t, m, "input")

	if count != base+1 {
		t.Errorf("expected %d rules with default log limit, got %d", base+1, count)
	}
}

// ---------------------------------------------------------------------------
// addCIDRAccept IPv6 early-return path (Docker custom network with IPv6 CIDR)
// ---------------------------------------------------------------------------

func TestIntegration_Apply_Docker_IPv6CIDR_SkippedGracefully(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	// IPv6 CIDR → addCIDRAccept returns early without adding a rule (not implemented).
	docker := shared.DockerConfig{
		Enabled:             true,
		AllowBridgeNetworks: false,
		CustomNetworks:      []string{"fd00::/8"},
	}
	if err := m.Apply(emptyState(), shared.FirewallOptions{}, shared.IPv6Config{}, docker); err != nil {
		t.Fatalf("Apply with IPv6 Docker CIDR: %v", err)
	}

	// No rule should be added because IPv6 CIDRs are silently skipped.
	count := ruleCount(t, m, "input")
	if count != base {
		t.Errorf("IPv6 Docker CIDR should be skipped: expected %d rules, got %d", base, count)
	}
}

func TestIntegration_Apply_Docker_IPv4CustomNetwork(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	docker := shared.DockerConfig{
		Enabled:             true,
		AllowBridgeNetworks: false,
		CustomNetworks:      []string{"192.168.100.0/24", "172.20.0.0/16"},
	}
	if err := m.Apply(emptyState(), shared.FirewallOptions{}, shared.IPv6Config{}, docker); err != nil {
		t.Fatalf("Apply with Docker custom networks: %v", err)
	}

	count := ruleCount(t, m, "input")
	if count != base+2 {
		t.Errorf("expected %d rules (base + 2 Docker CIDRs), got %d", base+2, count)
	}
}

// ---------------------------------------------------------------------------
// addCIDRDrop IPv6 CIDR path (blacklist with IPv6 CIDR — silently skipped)
// ---------------------------------------------------------------------------

func TestIntegration_Apply_Blacklist_IPv6CIDR_SkippedGracefully(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	// IPv6 CIDR in blacklist → addCIDRDrop returns early without a rule.
	state := emptyState()
	state.Current.Blacklist = []string{"2001:db8::/32"}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	count := ruleCount(t, m, "input")
	if count != base {
		t.Errorf("IPv6 CIDR blacklist should be skipped: expected %d rules, got %d", base, count)
	}
}

// ---------------------------------------------------------------------------
// addWhitelistRule plain IPv4/IPv6 paths
// ---------------------------------------------------------------------------

func TestIntegration_Apply_Whitelist_PlainIPv4(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	// Plain IPv4 address (not CIDR) exercises the ip4 != nil branch.
	state := emptyState()
	state.Current.Whitelist = []string{"10.0.0.1", "192.168.1.100"}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	count := ruleCount(t, m, "input")
	if count != base+2 {
		t.Errorf("expected %d rules (base + 2 plain IPv4 whitelist), got %d", base+2, count)
	}
}

func TestIntegration_Apply_Whitelist_PlainIPv6_SkippedGracefully(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	// IPv6 plain address → addWhitelistRule has no IPv6 implementation, silently skipped.
	state := emptyState()
	state.Current.Whitelist = []string{"2001:db8::1"}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	count := ruleCount(t, m, "input")
	if count != base {
		t.Errorf("IPv6 plain whitelist should be skipped: expected %d rules, got %d", base, count)
	}
}

// ---------------------------------------------------------------------------
// detectDockerBridges — create a real "docker0" dummy interface in the netns
// ---------------------------------------------------------------------------

func TestIntegration_DetectDockerBridges_WithInterface(t *testing.T) {
	// Create a dummy interface named docker0 with a Docker-range address.
	// Since we're running inside an isolated network namespace (TestMain),
	// this does not affect the host system at all.
	setup := [][]string{
		{"ip", "link", "add", "docker0", "type", "dummy"},
		{"ip", "addr", "add", "172.17.0.1/16", "dev", "docker0"},
		{"ip", "link", "set", "docker0", "up"},
	}
	for _, args := range setup {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Skipf("cannot set up docker0 interface: %v — %s", err, out)
		}
	}
	t.Cleanup(func() {
		exec.Command("ip", "link", "del", "docker0").Run() //nolint:errcheck
	})

	cidrs := detectDockerBridges()
	if len(cidrs) == 0 {
		t.Error("expected at least one CIDR from docker0 interface")
	}
	found := false
	for _, cidr := range cidrs {
		if cidr == "172.17.0.0/16" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 172.17.0.0/16 in detected bridges, got: %v", cidrs)
	}
}
