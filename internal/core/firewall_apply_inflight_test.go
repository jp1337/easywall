//go:build integration

package core

import (
	"errors"
	"testing"
	"time"

	"github.com/google/nftables"
	"github.com/jp1337/easywall/internal/shared"
)

// A second APPLY_RULES arriving while a window is open is refused, not queued.
//
// Queued was the previous behaviour and it inverted the whole point of the
// window: Apply serialised on a mutex it held for the entire acceptance window,
// so request two waited there and ran the moment request one rolled back —
// re-applying the rule set that had just been undone, and opening a second
// window for it. Measured before the fix, at a 2 s window: "acceptance timed
// out — rolling back" immediately followed by "starting rule apply".
//
// Integration-tagged because it needs an apply that actually reaches a kernel:
// with a stub manager the apply fails before the window opens, and the queue
// this is about never forms.
func TestIntegration_SecondApplyIsRefusedNotQueued(t *testing.T) {
	fw := newTestFirewallWithRealNft(t)
	defer func() {
		fw.nft.conn.DelTable(&nftables.Table{Name: tableName, Family: nftables.TableFamilyINet})
		_ = fw.nft.conn.Flush()
	}()
	fw.cfg.Acceptance.Duration = 2

	d := &Daemon{cfg: fw.cfg, firewall: fw, quit: make(chan struct{})}
	if err := fw.rules.SaveStaged("tcp", []shared.PortRule{{Port: "8080"}}); err != nil {
		t.Fatal(err)
	}

	if resp := d.dispatch(shared.Command{Type: shared.CmdApplyRules}); !resp.Success {
		t.Fatalf("first APPLY should start: %s", resp.Error)
	}
	waitForAcceptance(t, fw, shared.AcceptancePending)

	// The answer the web process gets has to be the truth. "started" for a
	// request that did not start is what let this ship.
	resp := d.dispatch(shared.Command{Type: shared.CmdApplyRules})
	if resp.Success {
		t.Error("a second APPLY was accepted while a window was open; it will run when " +
			"the window closes and re-apply rules the operator did not re-request")
	}
	if resp.Error != shared.ErrApplyInProgressText {
		t.Errorf("Response.Error = %q, want %q — the web process matches on this exact "+
			"string to say \"an apply is already running\"", resp.Error, shared.ErrApplyInProgressText)
	}

	// Direct callers get the sentinel rather than blocking for the window.
	if err := fw.Apply("probe"); !errors.Is(err, ErrApplyInProgress) {
		t.Errorf("Firewall.Apply during a window returned %v, want ErrApplyInProgress", err)
	}

	// Let the window expire, then confirm nothing started a second cycle.
	//
	// Deliberately not waiting for "rolled_back": Apply resets the status to idle
	// as it returns, so that value is transient and waiting for it is a race — it
	// failed 2 runs in 5. What this test is about is that no *second* window
	// opens, so wait for the first to be over and then look again.
	waitForAcceptanceCleared(t, fw)
	time.Sleep(500 * time.Millisecond)
	if got := fw.acceptance.Status(); got == shared.AcceptancePending {
		t.Error("a new acceptance window opened after the rollback: the queued apply ran")
	}

	// And a shutdown is not held for one further window per queued request.
	start := time.Now()
	d.Stop()
	if took := time.Since(start); took > time.Second {
		t.Errorf("Stop() took %s; with applies queued behind the open window it waited one "+
			"full window each, which at the shipped 120 s default outlasts systemd's "+
			"90 s TimeoutStopSec and gets the daemon killed mid-rollback", took)
	}
}

// The window is allowed again once the cycle has finished.
func TestIntegration_ApplyIsAllowedAgainAfterTheCycleEnds(t *testing.T) {
	fw := newTestFirewallWithRealNft(t)
	defer func() {
		fw.nft.conn.DelTable(&nftables.Table{Name: tableName, Family: nftables.TableFamilyINet})
		_ = fw.nft.conn.Flush()
	}()
	fw.cfg.Acceptance.Duration = 2

	d := &Daemon{cfg: fw.cfg, firewall: fw, quit: make(chan struct{})}
	if err := fw.rules.SaveStaged("tcp", []shared.PortRule{{Port: "8080"}}); err != nil {
		t.Fatal(err)
	}

	if resp := d.dispatch(shared.Command{Type: shared.CmdApplyRules}); !resp.Success {
		t.Fatalf("first APPLY: %s", resp.Error)
	}
	waitForAcceptance(t, fw, shared.AcceptancePending)
	if !fw.Accept() {
		t.Fatal("Accept found no open window")
	}
	waitForAcceptanceCleared(t, fw)

	if resp := d.dispatch(shared.Command{Type: shared.CmdApplyRules}); !resp.Success {
		t.Errorf("APPLY after a completed cycle was refused: %s — the slot was never released", resp.Error)
	}
	waitForAcceptance(t, fw, shared.AcceptancePending)
	fw.Accept()
	d.Stop()
}

func waitForAcceptance(t *testing.T, fw *Firewall, want shared.AcceptanceStatus) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if fw.acceptance.Status() == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("acceptance never reached %q (still %q)", want, fw.acceptance.Status())
}

// After a cycle ends the status returns to idle (Reset) or settles on a terminal
// value; either way it is no longer pending.
func waitForAcceptanceCleared(t *testing.T, fw *Firewall) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if fw.acceptance.Status() != shared.AcceptancePending {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("acceptance stayed pending")
}
