package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// auditEntries returns every entry in the test config's audit log, in the order
// it was written.
func auditEntries(t *testing.T, cfg *Config) []shared.AuditLogEntry {
	t.Helper()
	data, err := os.ReadFile(cfg.AuditLogPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	var entries []shared.AuditLogEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e shared.AuditLogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("audit line is not JSON: %q", line)
		}
		entries = append(entries, e)
	}
	return entries
}

// auditActions returns the action of every entry in the test config's audit log.
func auditActions(t *testing.T, cfg *Config) []string {
	t.Helper()
	var actions []string
	for _, e := range auditEntries(t, cfg) {
		actions = append(actions, e.Action)
	}
	return actions
}

// Panic mode is the whole point of the marker: a restore must not undo it.
func TestRestoreCurrent_SkipsWhenPanicIsEngaged(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	if err := EngagePanic(cfg.PanicMarkerPath()); err != nil {
		t.Fatalf("EngagePanic: %v", err)
	}

	if err := fw.RestoreCurrent(RestoreReasonBoot); err != nil {
		t.Errorf("a restore under panic mode must succeed by doing nothing, got %v", err)
	}
	if got := auditActions(t, cfg); len(got) != 0 {
		t.Errorf("a skipped restore must write no audit entry, got %v", got)
	}
}

// Without the marker the restore reaches nftables. The test firewall has a nil
// netlink connection, so it gets as far as Reset and fails there — which is the
// evidence that it tried, and the evidence that the failure is recorded rather
// than swallowed.
func TestRestoreCurrent_AttemptsAndRecordsFailure(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	err := fw.RestoreCurrent(RestoreReasonBoot)
	if err == nil {
		t.Fatal("with no netlink connection the restore must report a failure")
	}
	if !strings.Contains(err.Error(), "nftables connection not available") {
		t.Errorf("unexpected error: %v", err)
	}

	got := auditActions(t, cfg)
	if len(got) != 1 || got[0] != "boot_enforce_failed" {
		t.Errorf("want exactly one boot_enforce_failed entry, got %v", got)
	}
}

// The load-bearing assertion of the release. A restore must never open an
// acceptance window: nobody is present at boot to confirm it, so a window would
// expire, roll back to `backup`, and leave a set nobody confirmed either.
func TestRestoreCurrent_NeverOpensAnAcceptanceWindow(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Acceptance.Enabled = true
	cfg.Acceptance.Duration = 3600
	fw := newTestFirewall(t, cfg)

	_ = fw.RestoreCurrent(RestoreReasonBoot)

	if got := fw.acceptance.Status(); got != shared.AcceptanceIdle {
		t.Errorf("acceptance status after a restore = %q, want %q", got, shared.AcceptanceIdle)
	}
}

// A restore takes the same slot an apply does, so the two cannot both be
// writing the table.
func TestRestoreCurrent_RefusedWhileAnApplyHoldsTheSlot(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	if !fw.beginApply() {
		t.Fatal("could not claim the apply slot in the test")
	}
	defer fw.endApply()

	if err := fw.RestoreCurrent(RestoreReasonBoot); err != ErrApplyInProgress {
		t.Errorf("want ErrApplyInProgress, got %v", err)
	}
}

func TestFirewall_PanicEngagedReflectsTheMarker(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	if fw.PanicEngaged() {
		t.Error("a fresh install is not in panic mode")
	}
	if err := EngagePanic(cfg.PanicMarkerPath()); err != nil {
		t.Fatalf("EngagePanic: %v", err)
	}
	if !fw.PanicEngaged() {
		t.Error("the marker is there; PanicEngaged must say so")
	}
}

// A restore that cannot read the rules must record the failure. The audit log
// is what the operator looks at when the interface says the firewall is off.
func TestRestoreCurrent_RecordsAFailureToReadTheRules(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	// Corrupt the rules file so GetState fails
	if err := os.WriteFile(cfg.RulesPath(), []byte("not json"), 0600); err != nil {
		t.Fatalf("corrupt rules file: %v", err)
	}

	err := fw.RestoreCurrent(RestoreReasonBoot)
	if err == nil {
		t.Fatal("with corrupted rules the restore must fail")
	}

	got := auditActions(t, cfg)
	if len(got) != 1 || got[0] != "boot_enforce_failed" {
		t.Errorf("want exactly one boot_enforce_failed entry, got %v", got)
	}
}

// An apply that cannot read the rules must record the failure. The audit log
// is what the operator looks at when the interface says the firewall is off.
func TestApply_RecordsAFailureToReadTheRules(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Acceptance.Enabled = false
	fw := newTestFirewall(t, cfg)

	// Corrupt the rules file so GetState fails
	if err := os.WriteFile(cfg.RulesPath(), []byte("not json"), 0600); err != nil {
		t.Fatalf("corrupt rules file: %v", err)
	}

	err := fw.Apply("test")
	if err == nil {
		t.Fatal("with corrupted rules the apply must fail")
	}

	got := auditActions(t, cfg)
	if len(got) != 1 || got[0] != "apply_failed" {
		t.Errorf("want exactly one apply_failed entry, got %v", got)
	}
}

// Panic writes the marker even when tearing down the table fails. The order
// matters: an operator who ran `panic` and then rebooted must not come back to
// the rules that made them run it.
func TestPanic_WritesTheMarkerEvenWhenTeardownFails(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg) // nil netlink connection: Reset will fail

	err := fw.Panic("console")
	if err == nil {
		t.Fatal("with no netlink connection the teardown must be reported")
	}
	if !PanicEngaged(cfg.PanicMarkerPath()) {
		t.Error("the marker must be written even when the teardown failed")
	}
	if got := auditActions(t, cfg); len(got) != 1 || got[0] != "panic_engaged" {
		t.Errorf("want one panic_engaged entry, got %v", got)
	}
}

// Panic during an open acceptance window has to end the window without
// letting the apply goroutine waiting on it roll back on top of the
// teardown.
//
// The previous version of this test started a window with nothing ever
// calling Wait() on it, and asserted only that the status changed. That
// passed because Panic called f.acceptance.Reset() directly — a call with no
// purpose except making this assertion true, since a plain Reset() sets the
// status to idle regardless of anything CancelAcceptance did. The assertion
// would have passed with CancelAcceptance() deleted from Panic entirely, and
// deleting it is exactly the bug this whole task exists to fix. This version
// puts a real goroutine in Wait() — the way an apply actually does — and
// follows it all the way to the rollback that Wait() returning false
// triggers, which is where the actual protection now lives.
func TestPanic_EndsAnOpenAcceptanceWindow(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	if err := fw.acceptance.Start(cfg.AcceptanceDuration()); err != nil {
		t.Fatalf("start acceptance: %v", err)
	}
	if got := fw.acceptance.Status(); got != shared.AcceptancePending {
		t.Fatalf("precondition: acceptance status = %q", got)
	}

	// Stands in for the tail of Firewall.apply: a goroutine blocked in
	// Wait(), which on a cancel writes apply_rolledback and calls rollback —
	// exactly what apply() does when its window is not accepted.
	done := make(chan struct{})
	go func() {
		defer close(done)
		if accepted := fw.acceptance.Wait(); !accepted {
			WriteAuditLog(cfg.AuditLogPath(), "apply_rolledback", "all", "timeout", "apply-goroutine")
			fw.rollback(shared.RulesState{}, "apply-goroutine")
		}
	}()

	if err := fw.Panic("console"); err == nil {
		t.Fatal("with no netlink connection Panic's own teardown must be reported as failed")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the apply goroutine never returned from Wait(); Panic did not end the window")
	}

	if got := fw.acceptance.Status(); got == shared.AcceptancePending {
		t.Error("panic must not leave an acceptance window open")
	}

	actions := auditActions(t, cfg)
	if len(actions) == 0 || actions[0] != "panic_engaged" {
		t.Fatalf("want panic_engaged written first, got %v", actions)
	}
	var sawSkip bool
	for _, a := range actions {
		switch a {
		case "rollback_skipped":
			sawSkip = true
		case "rollback_failed":
			// rollback_failed would mean the rollback goroutine reached
			// nft.Apply(previous) and raced Panic's own nft.Reset() on the
			// same connection — the exact defect this task fixes.
			t.Errorf("the rollback attempted to touch nftables after panic mode was engaged; got %v", actions)
		}
	}
	if !sawSkip {
		t.Errorf("want a rollback_skipped entry once panic mode is engaged, got %v", actions)
	}
}

// A second apply — another browser tab, a request queued before the console
// ran `panic`, a client that has not been told yet — must not be able to
// re-arm a firewall the console just disarmed.
func TestApply_RefusedWhilePanicIsEngaged(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	if err := EngagePanic(cfg.PanicMarkerPath()); err != nil {
		t.Fatalf("EngagePanic: %v", err)
	}

	err := fw.apply("web")
	if !errors.Is(err, ErrPanicEngaged) {
		t.Errorf("apply() during panic mode = %v, want ErrPanicEngaged", err)
	}

	got := auditActions(t, cfg)
	if len(got) != 1 || got[0] != "apply_refused_panic" {
		t.Errorf("want exactly one apply_refused_panic entry, got %v", got)
	}
}

// The exported Apply must refuse the same way — beginApply claims the slot
// and then apply() finds the marker, so the slot has to come back too, or a
// refused apply would wedge every apply after it.
func TestApply_ExportedRefusesWhilePanicIsEngagedAndReleasesTheSlot(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	if err := EngagePanic(cfg.PanicMarkerPath()); err != nil {
		t.Fatalf("EngagePanic: %v", err)
	}

	if err := fw.Apply("web"); !errors.Is(err, ErrPanicEngaged) {
		t.Errorf("Apply() during panic mode = %v, want ErrPanicEngaged", err)
	}
	if !fw.beginApply() {
		t.Error("the apply slot was not released after a refusal; no further apply could ever run")
	}
	fw.endApply()
}

// Resume ends panic mode unconditionally, but the restore that follows can
// lose the apply slot to a cycle that is already running. That path used to
// write nothing: the operator was left with a log that said panic mode had
// ended and no line anywhere saying the rules never came back.
func TestResume_RecordsWhenAnApplyHoldsTheSlot(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	if err := EngagePanic(cfg.PanicMarkerPath()); err != nil {
		t.Fatalf("EngagePanic: %v", err)
	}
	if !fw.beginApply() {
		t.Fatal("could not claim the apply slot in the test")
	}
	defer fw.endApply()

	err := fw.Resume("console")
	if !errors.Is(err, ErrApplyInProgress) {
		t.Errorf("Resume() while an apply holds the slot = %v, want ErrApplyInProgress", err)
	}
	if PanicEngaged(cfg.PanicMarkerPath()) {
		t.Error("Resume must clear the marker even when the restore behind it cannot run")
	}

	got := auditActions(t, cfg)
	if len(got) != 2 || got[0] != "panic_resumed" || got[1] != "resume_restore_skipped" {
		t.Errorf("want [panic_resumed resume_restore_skipped], got %v", got)
	}
}

// Resume clears the marker first and only then restores, so a restore that fails
// does not leave the machine claiming to be in panic mode when it is not.
func TestResume_ClearsTheMarker(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	if err := EngagePanic(cfg.PanicMarkerPath()); err != nil {
		t.Fatalf("EngagePanic: %v", err)
	}

	_ = fw.Resume("console") // the restore fails on the nil connection; the clear must not

	if PanicEngaged(cfg.PanicMarkerPath()) {
		t.Error("resume must clear the marker even when the restore that follows fails")
	}
	got := auditActions(t, cfg)
	if len(got) == 0 || got[0] != "panic_resumed" {
		t.Errorf("want panic_resumed first, got %v", got)
	}
}

// A rollback interrupted by panic mode must still revert the rules file.
//
// This is the invariant RestoreCurrent's missing acceptance window rests on:
// Current is a set that has already survived a window. The panic guard used to
// return before the file revert as well as before the kernel write, which left
// Current holding the set the operator had just been cut off by — unconfirmed,
// equal to Staged so the dashboard reported nothing outstanding, and reinstalled
// with no window by the next `resume`.
func TestRollback_UnderPanicStillRevertsTheRulesFile(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	// The state after a confirmed apply: Current is the set that works.
	works := []shared.PortRule{{Port: "22", Description: "ssh"}}
	if err := fw.rules.SaveStaged("tcp", works); err != nil {
		t.Fatalf("stage the working rules: %v", err)
	}
	if err := fw.rules.BackupCurrent(); err != nil {
		t.Fatalf("backup: %v", err)
	}
	if err := fw.rules.PromoteStaged(); err != nil {
		t.Fatalf("promote: %v", err)
	}

	// The apply that cuts the operator off: staged, backed up, promoted, window
	// open. `previous` is what Firewall.apply captures before promoting.
	locksMeOut := []shared.PortRule{{Port: "9999", Description: "not ssh"}}
	if err := fw.rules.SaveStaged("tcp", locksMeOut); err != nil {
		t.Fatalf("stage the bad rules: %v", err)
	}
	previous, err := fw.rules.GetState()
	if err != nil {
		t.Fatalf("read the pre-apply state: %v", err)
	}
	if err := fw.rules.BackupCurrent(); err != nil {
		t.Fatalf("backup: %v", err)
	}
	if err := fw.rules.PromoteStaged(); err != nil {
		t.Fatalf("promote: %v", err)
	}

	// The operator reaches the console and runs `panic`.
	if err := EngagePanic(cfg.PanicMarkerPath()); err != nil {
		t.Fatalf("EngagePanic: %v", err)
	}

	fw.rollback(previous, "web")

	state, err := fw.rules.GetState()
	if err != nil {
		t.Fatalf("read the state after the rollback: %v", err)
	}
	if len(state.Current.TCP) != 1 || state.Current.TCP[0].Port != "22" {
		t.Errorf("Current after a panic-interrupted rollback = %+v, want the pre-apply set "+
			"(port 22); an unconfirmed set left in Current is restored with no acceptance "+
			"window at the next boot or resume", state.Current.TCP)
	}

	pending, err := fw.rules.HasPendingChanges()
	if err != nil {
		t.Fatalf("HasPendingChanges: %v", err)
	}
	if !pending {
		t.Error("HasPendingChanges must be true again after the revert — the operator's " +
			"staged correction is still waiting, and the dashboard has to say so")
	}

	// The kernel half must not have been attempted: the test firewall has a nil
	// netlink connection, so an attempt shows up as rollback_failed.
	var sawSkip bool
	for _, e := range auditEntries(t, cfg) {
		switch e.Action {
		case "rollback_skipped":
			sawSkip = true
			if !strings.Contains(e.Detail, cfg.PanicMarkerPath()) {
				t.Errorf("the rollback_skipped detail must name the marker that caused it, got %q", e.Detail)
			}
		case "rollback_failed":
			t.Errorf("the rollback touched nftables while panic mode was engaged: %q", e.Detail)
		}
	}
	if !sawSkip {
		t.Error("a rollback that left the kernel torn down must say so in the audit log")
	}
}

// lockedMarkerFirewall returns a Firewall whose panic marker cannot be read
// while its rules file and audit log still can.
//
// The three normally live in the same two directories, so a plain chmod would
// make every one of them unreadable and the test would prove nothing about the
// marker in particular. The rules store is therefore built from the good path
// first and DataDir is repointed afterwards: NewRulesStore keeps the path it was
// given, and DataDir is read again only by PanicMarkerPath.
func lockedMarkerFirewall(t *testing.T) (*Firewall, *Config) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root: a 0000 directory is still traversable")
	}
	cfg := newTestConfig(t)
	store, err := NewRulesStore(cfg.RulesPath())
	if err != nil {
		t.Fatalf("NewRulesStore: %v", err)
	}
	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) })
	cfg.DataDir = locked

	if _, known, _ := PanicState(cfg.PanicMarkerPath()); known {
		t.Skip("this filesystem lets the marker be stat'ed anyway; nothing to test")
	}
	return &Firewall{
		cfg:        cfg,
		nft:        &NftablesManager{},
		rules:      store,
		acceptance: NewAcceptance(cfg.AcceptanceDuration()),
	}, cfg
}

// An unreadable marker must not withdraw the acceptance window's automatic undo.
//
// PanicEngaged reads "cannot tell" as "engaged", which is the safe default for
// the boot restore and the wrong one here: a permission fault on the data
// directory would silently turn a firewall that always lets you back in into one
// that does not, and the audit entry would blame a decision at the console.
func TestRollback_ProceedsWhenTheMarkerCannotBeRead(t *testing.T) {
	fw, cfg := lockedMarkerFirewall(t)

	fw.rollback(shared.RulesState{}, "web")

	for _, e := range auditEntries(t, cfg) {
		if e.Action == "rollback_skipped" {
			t.Errorf("an unreadable marker must not be reported as an engaged one: %q", e.Detail)
		}
	}
	// The kernel write was attempted: the test firewall has a nil netlink
	// connection, so the attempt is what produces rollback_failed.
	var tried bool
	for _, e := range auditEntries(t, cfg) {
		if e.Action == "rollback_failed" && strings.Contains(e.Detail, "nftables") {
			tried = true
		}
	}
	if !tried {
		t.Error("the rollback must reach nftables when the marker state is unknown")
	}
}

// The fault is reported once, loudly, where an operator can act on it: an
// unreadable marker means the machine comes up unfiltered while every surface
// that shows panic mode shows one boolean and cannot tell the two apart.
func TestDaemonStart_RecordsAMarkerItCannotRead(t *testing.T) {
	fw, cfg := lockedMarkerFirewall(t)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	go func() { _ = d.Start() }()
	t.Cleanup(d.Stop)
	waitForSocket(t, cfg.SocketPath)

	entries := auditEntries(t, cfg)
	if len(entries) == 0 || entries[0].Action != "boot_enforce_failed" {
		t.Fatalf("want boot_enforce_failed first, got %v", auditActions(t, cfg))
	}
	if !strings.Contains(entries[0].Detail, cfg.PanicMarkerPath()) ||
		!strings.Contains(entries[0].Detail, "cannot read the panic marker") {
		t.Errorf("the entry must name the marker and the errno, got %q", entries[0].Detail)
	}
}

// The post-write marker check, on the two things a unit test can see: the
// verdict and the audit entry. That a real table actually comes down is the
// integration half — see TestIntegration_PanicLandingAfterAWriteLeavesNoRules,
// because this fixture has a nil netlink connection and nft.Apply never gets far
// enough to reach the check on its own.
func TestPanicLandedDuringWrite_RecordsAndReportsTheTeardown(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)

	if fw.panicLandedDuringWrite("boot_enforce_failed", "no marker here", "core") {
		t.Error("with no marker on disk the write that just happened stands")
	}
	if got := auditActions(t, cfg); len(got) != 0 {
		t.Errorf("a write nobody interrupted must record nothing here, got %v", got)
	}

	if err := EngagePanic(cfg.PanicMarkerPath()); err != nil {
		t.Fatalf("EngagePanic: %v", err)
	}

	if !fw.panicLandedDuringWrite("boot_enforce_failed", "boot: the console got there first", "core") {
		t.Fatal("a marker that appeared during the write must be reported")
	}
	entries := auditEntries(t, cfg)
	if len(entries) != 1 || entries[0].Action != "boot_enforce_failed" {
		t.Fatalf("want one boot_enforce_failed entry, got %v", auditActions(t, cfg))
	}
	if !strings.Contains(entries[0].Detail, "boot: the console got there first") {
		t.Errorf("the caller's detail must survive, got %q", entries[0].Detail)
	}
	// The fixture cannot reach nftables, and a teardown that did not happen is
	// the one thing an operator has to be told about: the machine may still be
	// filtering while the marker says it is not.
	if !strings.Contains(entries[0].Detail, "could not be torn down") {
		t.Errorf("a failed teardown must be named in the entry, got %q", entries[0].Detail)
	}
	if entries[0].User != "core" {
		t.Errorf("user = %q, want core", entries[0].User)
	}
}
