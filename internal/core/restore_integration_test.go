//go:build integration

package core

import (
	"strings"
	"testing"

	"github.com/google/nftables"
	"github.com/jp1337/easywall/internal/shared"
)

// The rest of the integration suite proves what a running daemon can build.
// 2.7's whole claim is about what happens to that ruleset when the daemon
// stops existing and starts again — and every test so far runs against a
// connection that was never actually severed. These two are the ones that
// sever it.
//
// Requires CAP_NET_ADMIN / root — run with:
//
//	sudo go test -tags integration ./internal/core/ -run 'AfterTheTableIsFlushed|PanicModeSurvives' -v

// TestIntegration_RulesAreBackAfterTheTableIsFlushed is the test for the
// release. A reboot, as far as nftables is concerned, is not a signal or an
// event this code can observe — it is the table simply not being there any
// more. So the test does exactly that: applies a rule set, deletes
// `table inet easywall` out from under it the way a reboot would, and calls
// the same restore the daemon calls on the way up.
//
// The assertion at the end is on meaning, not on count. This project's own
// history (see nftables_semantics_test.go) is that a suite which only counts
// rules will pass a ruleset that drops where it should accept, or restores
// the wrong ports entirely — a restore that came back with an empty table
// and a restore that came back with the *previous* installation's ports
// would both satisfy "the rule count is right".
func TestIntegration_RulesAreBackAfterTheTableIsFlushed(t *testing.T) {
	// newIntegrationManager's own connection is discarded as soon as it has
	// done its job of skipping this test where nftables is not reachable —
	// NewFirewall below opens the connection this test actually exercises.
	_ = newIntegrationManager(t)

	cfg := newTestConfig(t)
	cfg.Acceptance.Enabled = false // this test is about the restore, not the confirmation window

	fw, err := NewFirewall(cfg)
	if err != nil {
		t.Fatalf("NewFirewall: %v", err)
	}
	t.Cleanup(func() { _ = fw.nft.Reset() })

	// A rule set that says something specific: 9101 is open, 9102 is not
	// mentioned at all and therefore falls to the drop policy.
	if err := fw.rules.SaveStaged("tcp", []shared.PortRule{{Port: "9101", Description: "open"}}); err != nil {
		t.Fatalf("SaveStaged: %v", err)
	}
	if err := fw.Apply("test"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !fw.nft.Enforcing() {
		t.Fatal("precondition failed: Enforcing() is false right after an accepted Apply, " +
			"so the rest of this test would prove nothing")
	}

	// The reboot. Nothing about a restart is simulated here beyond the one
	// fact that matters to nftables: the table it was told to keep is gone.
	fw.nft.conn.DelTable(&nftables.Table{Name: tableName, Family: nftables.TableFamilyINet})
	if err := fw.nft.conn.Flush(); err != nil {
		t.Fatalf("flush the table deletion: %v", err)
	}
	if fw.nft.Enforcing() {
		t.Fatal("precondition failed: Enforcing() is still true after the table was deleted, " +
			"so this test's 'reboot' did not happen")
	}

	// What 2.7 adds: the daemon's startup path, exercised the same way
	// daemon.go and dockerreconcile.go call it.
	if err := fw.RestoreCurrent(RestoreReasonBoot); err != nil {
		t.Fatalf("RestoreCurrent: %v", err)
	}
	if !fw.nft.Enforcing() {
		t.Error("Enforcing() is false after RestoreCurrent; the release's central claim — " +
			"that the firewall comes back after a reboot — does not hold")
	}

	// And it has to be *these* rules, not merely some rules. A restore that
	// came back empty, or with the wrong port open, would still leave
	// Enforcing() true.
	dump := ruleset(t)
	if !strings.Contains(dump, "9101") {
		t.Errorf("the restored ruleset does not mention port 9101, which was staged and applied "+
			"before the reboot — RestoreCurrent put something back, but not what was stored:\n%s", dump)
	}
	if strings.Contains(dump, "9102") {
		t.Errorf("the restored ruleset mentions port 9102, which was never staged at all — "+
			"RestoreCurrent must not invent rules that were never asked for:\n%s", dump)
	}
}

// TestIntegration_PanicModeSurvivesARestart is the test for the escape route
// 2.7 could otherwise remove by accident. Before this release, an operator
// locked out by their own rules got the firewall back by rebooting — nftables
// came up empty and stayed that way until someone pressed Apply. Restoring
// automatically at boot closes that door; panic mode is what 2.7 opens in its
// place, and this is the proof that the replacement survives the same event
// the original escape route relied on.
//
// A second *Firewall built over the same cfg — same DataDir, same on-disk
// panic marker — is deliberately used in place of the first one: that is what
// NewDaemon actually builds after a process restart, and nothing about the
// first Firewall's in-memory state (which holds no notion of panic mode at
// all; PanicEngaged always re-reads the marker file) is carried across on a
// real reboot either.
func TestIntegration_PanicModeSurvivesARestart(t *testing.T) {
	_ = newIntegrationManager(t)

	cfg := newTestConfig(t)
	cfg.Acceptance.Enabled = false

	fw, err := NewFirewall(cfg)
	if err != nil {
		t.Fatalf("NewFirewall: %v", err)
	}
	t.Cleanup(func() { _ = fw.nft.Reset() })
	// Whatever else happens in this test, the panic marker must not survive
	// it. Firewall.apply refuses outright while it is set and Firewall.rollback
	// returns early, so a marker left behind here would make some later test
	// in this package behave as though the console had taken the firewall
	// down — and that test's failure would point nowhere near this one.
	t.Cleanup(func() { _ = ClearPanic(cfg.PanicMarkerPath()) })

	if err := fw.rules.SaveStaged("tcp", []shared.PortRule{{Port: "9101", Description: "open"}}); err != nil {
		t.Fatalf("SaveStaged: %v", err)
	}
	if err := fw.Apply("test"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := fw.Panic("test"); err != nil {
		t.Fatalf("Panic: %v", err)
	}
	if fw.nft.Enforcing() {
		t.Fatal("precondition failed: Enforcing() is still true after Panic, " +
			"so this test's premise — a deliberately unfiltered machine — does not hold")
	}

	// The restart: a second Firewall over the same data directory, built the
	// way the daemon builds one after a process restart. Its NftablesManager
	// holds its own connection and its own mutex — that is fine here, the two
	// Firewalls are never touched concurrently — but nothing about fw's
	// in-memory state crosses over; only what is on disk does.
	restarted, err := NewFirewall(cfg)
	if err != nil {
		t.Fatalf("NewFirewall after restart: %v", err)
	}
	t.Cleanup(func() { _ = restarted.nft.Reset() })

	if err := restarted.RestoreCurrent(RestoreReasonBoot); err != nil {
		t.Fatalf("RestoreCurrent: %v", err)
	}
	if restarted.nft.Enforcing() {
		t.Error("Enforcing() is true after RestoreCurrent on the post-restart Firewall, although " +
			"the panic marker was on disk before this Firewall was even built — panic mode did not " +
			"survive the restart, and the operator who took the firewall down would meet it again")
	}

	// And the operator's way out of panic mode has to work after a restart
	// too, on the same Firewall value that just refused to restore.
	if err := restarted.Resume("test"); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if !restarted.nft.Enforcing() {
		t.Error("Enforcing() is false after Resume; ending panic mode must put the stored rules back")
	}
}
