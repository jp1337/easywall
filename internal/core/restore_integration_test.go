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

	// And it has to be *these* rules, with *this* verdict — not merely some
	// rules that happen to name the right port. "tcp dport 9101" alone would
	// still be found in a rule reading `tcp dport 9101 drop`, which is the
	// exact inversion this test exists to catch; the accept has to be part of
	// what is matched, the same way the semantics tests elsewhere in this
	// package assert "...accept" and "...drop" rather than the address alone.
	dump := ruleset(t)
	mustContain(t, dump, "tcp dport 9101 accept",
		"port 9101 was staged and applied before the reboot, and RestoreCurrent must put back "+
			"a rule that accepts it — not merely one that mentions the number 9101")
	// Unqualified on purpose: 9102 was never staged, so no rule of any kind —
	// accept, drop, or otherwise — should name it anywhere in the table.
	// Qualifying this one with a verdict would only make it easier to pass by
	// accident; the correct claim is total absence, and that is a stronger
	// check than "absent with the wrong verdict" would be.
	mustNotContain(t, dump, "9102",
		"RestoreCurrent must not invent rules for a port that was never staged")
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

// A panic that lands *during* a write to the table has to leave the table down.
//
// Every panic check in this package used to run before a write and none after
// one, so the loser of that race was a machine filtering with the marker on
// disk — reported by `easywall-core status` and the web banner as "deliberately
// not enforcing", because both test the panic flag before the enforcing one.
// Firewall.panicLandedDuringWrite closes it by re-reading the marker once
// nft.Apply has returned and tearing the table down again if it has appeared.
//
// The unit half of this (the verdict and the audit entry) is
// TestPanicLandedDuringWrite_RecordsAndReportsTheTeardown. What needs a real
// kernel is the teardown itself: that after this runs there is no ruleset left,
// not merely that a function returned true.
//
// The window is not opened artificially here — a marker cannot be made to appear
// between two adjacent statements from outside the process without a hook this
// code deliberately does not have. So the test writes the marker after a genuine
// apply has put real rules in the kernel and then runs the check exactly where
// apply and RestoreCurrent run it, which is the same state those two are in when
// they call it.
//
//	sudo go test -tags integration ./internal/core/ -run 'PanicLandingAfterAWrite' -v
func TestIntegration_PanicLandingAfterAWriteLeavesNoRules(t *testing.T) {
	_ = newIntegrationManager(t)

	cfg := newTestConfig(t)
	cfg.Acceptance.Enabled = false // the confirmation window is not what this is about

	fw, err := NewFirewall(cfg)
	if err != nil {
		t.Fatalf("NewFirewall: %v", err)
	}
	t.Cleanup(func() { _ = fw.nft.Reset() })

	if err := fw.rules.SaveStaged("tcp", []shared.PortRule{{Port: "9101", Description: "open"}}); err != nil {
		t.Fatalf("SaveStaged: %v", err)
	}
	if err := fw.Apply("test"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !fw.nft.Enforcing() {
		t.Fatal("precondition failed: Enforcing() is false right after an accepted Apply, " +
			"so the teardown below would prove nothing")
	}
	mustContain(t, ruleset(t), "tcp dport 9101 accept",
		"the rules this test is about to see taken down have to be there first")

	// The console: `easywall-core panic` with no daemon answering writes the
	// marker and tears the table down itself. This is the ordering where its
	// teardown loses — it lands before the daemon's write, so the daemon's write
	// is what the kernel ends up holding.
	if err := EngagePanic(cfg.PanicMarkerPath()); err != nil {
		t.Fatalf("EngagePanic: %v", err)
	}

	if !fw.panicLandedDuringWrite("boot_enforce_failed", "test: the console got there first", "core") {
		t.Fatal("the marker is on disk; the check has to see it")
	}
	if fw.nft.Enforcing() {
		t.Error("Enforcing() is true after a panic that landed during the write — the machine " +
			"is filtering while every status surface reports panic mode, which is the one " +
			"outcome panic mode exists to rule out")
	}
	mustNotContain(t, ruleset(t), "9101",
		"no rule from the interrupted write may survive; the table is supposed to be empty")

	entries := auditEntries(t, cfg)
	if len(entries) == 0 {
		t.Fatal("the interrupted write recorded nothing")
	}
	last := entries[len(entries)-1]
	if last.Action != "boot_enforce_failed" {
		t.Errorf("last action = %q, want boot_enforce_failed", last.Action)
	}
	if !strings.Contains(last.Detail, "the table was taken down again") {
		t.Errorf("the detail must record the teardown that actually happened, got %q", last.Detail)
	}
	for _, e := range entries {
		if e.Action == "boot_enforced" {
			t.Error("boot_enforced must not be the story of a machine that ended up unfiltered")
		}
	}
}
