package web

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTOTPReplay_TheSameStepTwiceIsRefused(t *testing.T) {
	r := newTOTPReplay(filepath.Join(t.TempDir(), "totp_replay.json"))

	if !r.accept(58123456) {
		t.Fatal("the first use of a step was refused")
	}
	if r.accept(58123456) {
		t.Error("the same step was accepted twice; a code intercepted in the last 30 seconds is replayable")
	}
}

// Store the step that matched, not the current one. A code accepted at N-1 must
// leave N usable — otherwise the operator is locked out for thirty seconds
// immediately after a successful login, which reads as the factor being broken.
func TestTOTPReplay_AcceptingAnEarlierStepLeavesTheCurrentOneValid(t *testing.T) {
	r := newTOTPReplay(filepath.Join(t.TempDir(), "totp_replay.json"))

	if !r.accept(58123455) {
		t.Fatal("N-1 was refused")
	}
	if !r.accept(58123456) {
		t.Error("accepting N-1 burned N")
	}
}

func TestTOTPReplay_TheStoredStepSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "totp_replay.json")

	first := newTOTPReplay(path)
	if !first.accept(58123456) {
		t.Fatal("the first use was refused")
	}

	// A new process, the same file.
	second := newTOTPReplay(path)
	if got := second.last(); got != 58123456 {
		t.Errorf("after a restart the last accepted step is %d, want 58123456", got)
	}
	if second.accept(58123456) {
		t.Error("a restart made an already-used code usable again")
	}
}

// The file is in data_dir, which the web user owns. A directory that cannot be
// written must not fail a login: the replay guard is a hardening measure, and
// refusing to sign anybody in because a disk is full is the worse outcome.
func TestTOTPReplay_AnUnwritableStoreDoesNotRefuseTheLogin(t *testing.T) {
	// Permission bits do not stop root. Under root the chmod below changes
	// nothing, the write succeeds, and the assertion passes while proving
	// nothing at all — accept() returns true either way, which is exactly what
	// makes this test's premise unfalsifiable if it is not checked. A container
	// build step or a root CI runner is all it takes.
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod 0500 cannot make the write fail, so this test " +
			"is unable to create the condition it exists to check")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0500); err != nil {
		t.Skipf("cannot make the directory read-only here: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0500) })

	path := filepath.Join(dir, "totp_replay.json")
	r := newTOTPReplay(path)
	if !r.accept(58123456) {
		t.Error("a step was refused because the store could not be written")
	}

	// The premise, asserted rather than assumed. Without this the test stays
	// green on any system where the directory turns out to be writable after
	// all, and a green test that cannot fail is the same as no test.
	if _, err := os.Stat(path); err == nil {
		t.Fatal("the store was written after all, so this test proved nothing about " +
			"what happens when it cannot be")
	}
}

func TestTOTPReplay_AGarbageFileIsNotFatal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "totp_replay.json")
	if err := os.WriteFile(path, []byte("{{{ not json"), 0600); err != nil {
		t.Fatal(err)
	}
	r := newTOTPReplay(path)
	if got := r.last(); got != 0 {
		t.Errorf("a file that does not parse produced last = %d, want 0", got)
	}
	if !r.accept(58123456) {
		t.Error("a file that does not parse refused a login")
	}
}
