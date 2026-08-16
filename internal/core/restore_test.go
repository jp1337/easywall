package core

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// auditActions returns the action of every entry in the test config's audit log.
func auditActions(t *testing.T, cfg *Config) []string {
	t.Helper()
	data, err := os.ReadFile(cfg.AuditLogPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	var actions []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e shared.AuditLogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("audit line is not JSON: %q", line)
		}
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
