//go:build integration

package core

import (
	"encoding/json"
	"net"
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
// addCIDRAccept IPv6 path (Docker custom network with IPv6 CIDR)
// ---------------------------------------------------------------------------

func TestIntegration_Apply_Docker_IPv6CIDR(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	docker := shared.DockerConfig{
		Enabled:             true,
		AllowBridgeNetworks: false,
		CustomNetworks:      []string{"fd00::/8"},
	}
	if err := m.Apply(emptyState(), shared.FirewallOptions{}, shared.NetworkSettings{Docker: docker}); err != nil {
		t.Fatalf("Apply with IPv6 Docker CIDR: %v", err)
	}

	// IPv6 CIDR accept rule should be added.
	count := ruleCount(t, m, "input")
	if count != base+1 {
		t.Errorf("expected %d rules (base + 1 IPv6 Docker CIDR), got %d", base+1, count)
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
	if err := m.Apply(emptyState(), shared.FirewallOptions{}, shared.NetworkSettings{Docker: docker}); err != nil {
		t.Fatalf("Apply with Docker custom networks: %v", err)
	}

	count := ruleCount(t, m, "input")
	if count != base+2 {
		t.Errorf("expected %d rules (base + 2 Docker CIDRs), got %d", base+2, count)
	}
}

// ---------------------------------------------------------------------------
// addCIDRDrop IPv6 CIDR path (blacklist with IPv6 CIDR)
// ---------------------------------------------------------------------------

func TestIntegration_Apply_Blacklist_IPv6CIDR(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	state := emptyState()
	state.Current.Blacklist = []string{"2001:db8::/32"}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// IPv6 CIDR drop rule should be added.
	count := ruleCount(t, m, "input")
	if count != base+1 {
		t.Errorf("expected %d rules (base + 1 IPv6 CIDR blacklist), got %d", base+1, count)
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
	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	count := ruleCount(t, m, "input")
	if count != base+2 {
		t.Errorf("expected %d rules (base + 2 plain IPv4 whitelist), got %d", base+2, count)
	}
}

func TestIntegration_Apply_Whitelist_PlainIPv6(t *testing.T) {
	m := newIntegrationManager(t)
	base := baseInputRules(t, m)

	state := emptyState()
	state.Current.Whitelist = []string{"2001:db8::1"}
	if err := m.Apply(state, shared.FirewallOptions{}, shared.NetworkSettings{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// IPv6 single-address accept rule should be added.
	count := ruleCount(t, m, "input")
	if count != base+1 {
		t.Errorf("expected %d rules (base + 1 IPv6 whitelist), got %d", base+1, count)
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

// ---------------------------------------------------------------------------
// NewDaemon + Daemon.Start lifecycle
// ---------------------------------------------------------------------------

func TestIntegration_NewDaemon_Start_Stop(t *testing.T) {
	// Confirm nftables is reachable before creating the daemon (NewDaemon → NewFirewall).
	_ = newIntegrationManager(t)

	cfg := newTestConfig(t)

	d, err := NewDaemon(cfg)
	if err != nil {
		t.Fatalf("NewDaemon: %v", err)
	}

	// Start the daemon in a goroutine — it blocks until Stop is called.
	errCh := make(chan error, 1)
	go func() { errCh <- d.Start() }()

	// Wait for the socket to appear (Start calls net.Listen and then loops on Accept).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(cfg.SocketPath); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := os.Stat(cfg.SocketPath); err != nil {
		t.Fatalf("socket not created by Start: %v", err)
	}

	// Send a real GetStatus command to the running daemon over the socket.
	conn, err := net.DialTimeout("unix", cfg.SocketPath, 5*time.Second)
	if err != nil {
		t.Fatalf("dial daemon socket: %v", err)
	}
	cmd := shared.Command{Type: shared.CmdGetStatus}
	out, _ := json.Marshal(cmd)
	_, _ = conn.Write(out)
	if uc, ok := conn.(*net.UnixConn); ok {
		_ = uc.CloseWrite()
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	respData := make([]byte, 4096)
	n, _ := conn.Read(respData)
	conn.Close()

	var resp shared.Response
	if err := json.Unmarshal(respData[:n], &resp); err != nil {
		t.Fatalf("parse daemon response: %v", err)
	}
	if !resp.Success {
		t.Errorf("GetStatus returned failure: %s", resp.Error)
	}

	// Stop the daemon gracefully and verify Start returned nil.
	d.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Start did not return within 5s after Stop")
	}
}

// ---------------------------------------------------------------------------
// The acceptance switch
// ---------------------------------------------------------------------------

// acceptance.enabled sits on the system settings page, is documented as
// "Off — an apply is final. There is no automatic way back", and was read by
// nothing until 2.5.0. The window opened either way, so an operator who
// switched it off — on a machine they can physically reach, which is exactly
// who the setting is for — still had the change rolled back under them.
//
// This has to run against a real kernel: with no netlink connection Apply
// fails before it ever reaches the acceptance step, so a unit test would pass
// whatever the branch did.
func TestIntegration_Apply_AcceptanceDisabled_ReturnsWithoutWaiting(t *testing.T) {
	m := newIntegrationManager(t)
	cfg := newTestConfig(t)
	cfg.Acceptance.Enabled = false
	cfg.Acceptance.Duration = 3600 // an hour, if a window were opened

	store, err := NewRulesStore(cfg.RulesPath())
	if err != nil {
		t.Fatalf("NewRulesStore: %v", err)
	}
	fw := &Firewall{
		cfg:        cfg,
		nft:        m,
		rules:      store,
		acceptance: NewAcceptance(cfg.AcceptanceDuration()),
	}
	if err := store.SaveStaged("tcp", []shared.PortRule{{Port: "8080"}}); err != nil {
		t.Fatalf("SaveStaged: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- fw.Apply("test") }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Apply is waiting for a confirmation the operator switched off")
	}

	if got := fw.acceptance.Status(); got == shared.AcceptancePending {
		t.Errorf("acceptance status is %q; no window should have opened", got)
	}

	// The rules must be live, not rolled back.
	state, err := store.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if len(state.Current.TCP) != 1 || state.Current.TCP[0].Port != "8080" {
		t.Errorf("the applied rule did not survive: %+v", state.Current.TCP)
	}
	if fw.Status().LastApply == "" {
		t.Error("an apply that needs no confirmation is still an apply; " +
			"it must set the last-applied time")
	}
}

// And with the switch on, the window really does open.
func TestIntegration_Apply_AcceptanceEnabled_OpensTheWindow(t *testing.T) {
	m := newIntegrationManager(t)
	cfg := newTestConfig(t)
	cfg.Acceptance.Enabled = true
	cfg.Acceptance.Duration = 30

	store, err := NewRulesStore(cfg.RulesPath())
	if err != nil {
		t.Fatalf("NewRulesStore: %v", err)
	}
	fw := &Firewall{
		cfg:        cfg,
		nft:        m,
		rules:      store,
		acceptance: NewAcceptance(cfg.AcceptanceDuration()),
	}

	done := make(chan error, 1)
	go func() { done <- fw.Apply("test") }()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if fw.acceptance.Status() == shared.AcceptancePending {
			fw.Accept()
			<-done
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no acceptance window opened, although the switch is on")
}

// Stopping the daemon while an acceptance window is open must roll the rules
// back, not abandon them. A package upgrade or a systemctl restart in the two
// minutes after an apply is an ordinary event, and it used to make an
// unconfirmed rule set permanent.
func TestIntegration_StopDuringAnOpenWindowRollsBack(t *testing.T) {
	// A real netlink manager: with the stub, Apply fails before it ever reaches
	// the acceptance window, and the test would pass without exercising it.
	fw := newTestFirewallWithRealNft(t)
	cfg := fw.cfg
	cfg.Acceptance.Enabled = true
	cfg.Acceptance.Duration = 3600 // long enough that only the cancel can end it

	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	// Stage a change so the rollback has something visible to undo.
	if err := fw.rules.SaveStaged("tcp", []shared.PortRule{{Port: "4711"}}); err != nil {
		t.Fatalf("SaveStaged: %v", err)
	}

	resp := d.dispatch(shared.Command{Type: shared.CmdApplyRules})
	if !resp.Success {
		t.Fatalf("apply was not started: %s", resp.Error)
	}

	// Wait for the window to actually open before stopping.
	deadline := time.Now().Add(5 * time.Second)
	for fw.acceptance.Status() != shared.AcceptancePending {
		if time.Now().After(deadline) {
			t.Fatal("the acceptance window never opened")
		}
		time.Sleep(10 * time.Millisecond)
	}

	stopped := make(chan struct{})
	go func() { d.Stop(); close(stopped) }()

	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("Stop blocked on the open acceptance window instead of ending it")
	}

	// The status is reset to idle at the end of the cycle, so the thing to
	// assert is the effect: the unconfirmed change must be gone.
	state, err := fw.rules.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	for _, r := range state.Current.TCP {
		if r.Port == "4711" {
			t.Error("the unconfirmed rule is still current; the rollback did not run")
		}
	}
}

// The end-to-end version of the promise the apply-flow diagram makes on four
// pages: after a window expires, the previous rules are back and nothing staged
// was lost. Losing the edits is worst exactly here — the operator has just been
// cut off by a bad rule and has to redo the work over a link they have proved
// is fragile.
func TestIntegration_RollbackKeepsTheStagedEdits(t *testing.T) {
	fw := newTestFirewallWithRealNft(t)
	cfg := fw.cfg
	cfg.Acceptance.Enabled = true
	cfg.Acceptance.Duration = 10 // the minimum; the test cancels rather than waits

	// A working set, applied and confirmed.
	if err := fw.rules.SaveStaged("tcp", []shared.PortRule{{Port: "22"}}); err != nil {
		t.Fatal(err)
	}
	go func() {
		for fw.acceptance.Status() != shared.AcceptancePending {
			time.Sleep(5 * time.Millisecond)
		}
		fw.Accept()
	}()
	if err := fw.Apply("test"); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	// The edit that will be rolled back.
	edits := []shared.PortRule{{Port: "22"}, {Port: "8443", Description: "the work"}}
	if err := fw.rules.SaveStaged("tcp", edits); err != nil {
		t.Fatal(err)
	}

	go func() {
		for fw.acceptance.Status() != shared.AcceptancePending {
			time.Sleep(5 * time.Millisecond)
		}
		fw.acceptance.Cancel() // stands in for "nobody confirmed"
	}()
	if err := fw.Apply("test"); err != nil {
		t.Fatalf("second apply: %v", err)
	}

	state, err := fw.rules.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Current.TCP) != 1 || state.Current.TCP[0].Port != "22" {
		t.Errorf("the enforced set must be the previous one: %+v", state.Current.TCP)
	}
	if len(state.Staged.TCP) != 2 {
		t.Fatalf("the staged edits must survive the rollback: %+v", state.Staged.TCP)
	}
	if state.Staged.TCP[1].Description != "the work" {
		t.Errorf("the staged edit lost its content: %+v", state.Staged.TCP[1])
	}

	// The kernel must be back on the previous set too.
	rs := ruleset(t)
	mustContain(t, rs, "tcp dport 22 accept", "the previous rule is enforced again")
	mustNotContain(t, rs, "tcp dport 8443", "the rolled-back rule must be gone from the kernel")
}

// The snapshot follows the kernel. Every place nft.Apply succeeds records the
// configuration that went in with the rules, because "what is live" is otherwise
// unknowable — the options and the network settings are in this daemon's config
// file, which changes without the kernel changing.
func TestIntegration_AppliedConfigIsRecordedWhereverTheKernelIsWritten(t *testing.T) {
	fw := newTestFirewallWithRealNft(t)
	cfg := fw.cfg
	// Apply blocks for the whole acceptance window unless it is switched off;
	// this test cares about what got recorded, not about the confirmation flow.
	cfg.Acceptance.Enabled = false

	if _, err := os.Stat(cfg.AppliedConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("a fresh data directory already has a snapshot: %v", err)
	}

	// The restore path, which is what an upgrade to 2.10 runs at the first
	// service start.
	if err := fw.RestoreCurrent(RestoreReasonBoot); err != nil {
		t.Fatalf("RestoreCurrent: %v", err)
	}
	res, err := readAppliedConfig(cfg.AppliedConfigPath())
	if err != nil || !res.Recorded {
		t.Fatalf("a restore did not record the configuration: recorded=%v err=%v", res.Recorded, err)
	}

	// The apply path, with the configuration changed underneath it: the snapshot
	// must hold the new value, not the one the restore wrote.
	opts := cfg.FirewallOptions()
	opts.Fragments = !opts.Fragments
	if err := cfg.SaveFirewallOptions(opts); err != nil {
		t.Fatal(err)
	}
	if err := fw.Apply("test"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	res, err = readAppliedConfig(cfg.AppliedConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if drift := shared.DiffConfig(res.Config, shared.AppliedConfig{
		Firewall: cfg.FirewallOptions(), Network: cfg.NetworkSettings(),
	}); len(drift) != 0 {
		t.Errorf("after an apply the snapshot still disagrees with the live config: %v", drift)
	}
	if fw.Status().HasPending {
		t.Error("an apply that just recorded its own configuration still reports a pending change")
	}
}
