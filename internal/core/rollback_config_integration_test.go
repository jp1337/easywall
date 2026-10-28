//go:build integration

package core

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// A rollback undoes the configuration that went live with the apply, not only
// the rules. Measured before the fix: a confirmed apply with ipv6.mode=filter,
// mode=block saved, applied, rolled back by the operator — and the kernel still
// held `meta nfproto ipv6 drop`, under a status reading rolled_back. The rules
// file was reverted and the table rebuilt with the very option that had caused
// the rollback, so "doing nothing restores the previous set" was false for every
// option and network change.
func TestIntegration_ARollbackRestoresThePreviousOptions(t *testing.T) {
	fw := newTestFirewallWithRealNft(t)
	cfg := fw.cfg
	cfg.Acceptance.Duration = shared.AcceptanceDurationMin

	confirmedIPv6Filter(t, fw)

	blockIPv6(t, cfg)
	go func() {
		for fw.acceptance.Status() != shared.AcceptancePending || !fw.nft.Enforcing() {
			time.Sleep(5 * time.Millisecond)
		}
		time.Sleep(100 * time.Millisecond) // past the kernel write
		fw.Rollback()
	}()
	if err := fw.Apply("test"); err != nil {
		t.Fatalf("apply (to be rolled back): %v", err)
	}

	assertIPv6Filtered(t, fw, "after an operator rollback")
	if st := fw.Status(); st.Acceptance != shared.AcceptanceRolledBack {
		t.Errorf("status = %s, want rolled_back", st.Acceptance)
	}
}

// The same promise across a core that dies inside the window: the restart's
// revert must rebuild the table with the options that were live before that
// apply, not with the ones the operator was trying out.
func TestIntegration_ARestartInsideTheWindowRestoresThePreviousOptions(t *testing.T) {
	fw := newTestFirewallWithRealNft(t)
	cfg := fw.cfg

	confirmedIPv6Filter(t, fw)

	blockIPv6(t, cfg)
	done := make(chan struct{})
	go func() { _ = fw.Apply("test"); close(done) }()
	defer func() { fw.acceptance.Cancel(); <-done }()
	deadline := time.Now().Add(5 * time.Second)
	for fw.Status().Acceptance != shared.AcceptancePending || !fw.nft.Enforcing() {
		if time.Now().After(deadline) {
			t.Fatal("the apply never opened its window")
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond) // past the kernel write

	restarted, err := NewFirewall(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.RestoreCurrent(RestoreReasonBoot); err != nil {
		t.Fatalf("boot restore: %v", err)
	}
	assertIPv6Filtered(t, restarted, "after a restart inside the window")
}

// confirmedIPv6Filter makes a confirmed apply with ipv6.mode=filter and port 22.
func confirmedIPv6Filter(t *testing.T, fw *Firewall) {
	t.Helper()
	nets := fw.cfg.NetworkSettings()
	nets.IPv6.Mode = shared.IPv6Filter
	if err := fw.cfg.SaveNetworkSettings(nets); err != nil {
		t.Fatal(err)
	}
	if err := fw.rules.SaveStaged("tcp", []shared.PortRule{{Port: "22", SSH: true}}); err != nil {
		t.Fatal(err)
	}
	go func() {
		for fw.acceptance.Status() != shared.AcceptancePending {
			time.Sleep(5 * time.Millisecond)
		}
		fw.Accept()
	}()
	if err := fw.Apply("test"); err != nil {
		t.Fatalf("confirmed apply: %v", err)
	}
	if out := inputChain(t); strings.Contains(out, "meta nfproto ipv6 drop") {
		t.Fatalf("ipv6.mode=filter already drops IPv6:\n%s", out)
	}
}

func blockIPv6(t *testing.T, cfg *Config) {
	t.Helper()
	nets := cfg.NetworkSettings()
	nets.IPv6.Mode = shared.IPv6Block
	if err := cfg.SaveNetworkSettings(nets); err != nil {
		t.Fatal(err)
	}
}

// assertIPv6Filtered checks the kernel is back on ipv6.mode=filter, and that the
// operator's mode=block is still configured and shown as pending — staged, like
// a rolled-back rule edit, not discarded.
func assertIPv6Filtered(t *testing.T, fw *Firewall, when string) {
	t.Helper()
	if out := inputChain(t); strings.Contains(out, "meta nfproto ipv6 drop") {
		t.Errorf("%s the kernel still drops IPv6 — the rolled-back option stayed live:\n%s", when, out)
	}
	if got := fw.cfg.NetworkSettings().IPv6.Mode; got != shared.IPv6Block {
		t.Errorf("%s the configured ipv6.mode is %q; the operator's edit must survive", when, got)
	}
	res, err := readAppliedConfig(fw.cfg.AppliedConfigPath())
	if err != nil || !res.Recorded {
		t.Fatalf("%s no applied config: recorded=%v err=%v", when, res.Recorded, err)
	}
	if res.Config.Network.IPv6.Mode != shared.IPv6Filter {
		t.Errorf("%s the recorded applied config says ipv6.mode=%q, want filter", when, res.Config.Network.IPv6.Mode)
	}
	if !fw.Status().HasPending {
		t.Errorf("%s the rolled-back option change is not reported as pending", when)
	}
}

func inputChain(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("nft", "list", "chain", "inet", tableName, "input").CombinedOutput()
	if err != nil {
		t.Fatalf("nft list: %v: %s", err, out)
	}
	return string(out)
}
