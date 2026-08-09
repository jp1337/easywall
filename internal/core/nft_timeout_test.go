package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// hangingNft installs a stand-in for the nft binary that never returns, and a
// short timeout, restoring both afterwards.
func hangingNft(t *testing.T, timeout time.Duration) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "nft-hang")
	script := "#!/bin/sh\nsleep 300\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil { //nolint:gosec // a test fixture that must be executable
		t.Fatal(err)
	}

	oldBin, oldTimeout, oldDelay := nftBinary, nftTimeout, nftWaitDelay
	nftBinary, nftTimeout, nftWaitDelay = path, timeout, 300*time.Millisecond
	t.Cleanup(func() { nftBinary, nftTimeout, nftWaitDelay = oldBin, oldTimeout, oldDelay })
}

// applyCustomRules runs inside Firewall.Apply, which holds the apply mutex for
// the whole cycle. An nft that never returns therefore does not just fail one
// apply: it wedges every future one, and Stop waits on the same goroutine — a
// firewall manager that can no longer change the firewall and cannot be shut
// down either. It had no timeout at all.
func TestApplyCustomRules_TimesOutInsteadOfHanging(t *testing.T) {
	hangingNft(t, 200*time.Millisecond)

	m := &NftablesManager{}
	start := time.Now()
	err := m.applyCustomRules([]string{"tcp dport 9999 accept"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a hung nft must produce an error, not a successful apply")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("the error should say it timed out, got: %v", err)
	}
	// The sleep in the stand-in runs for five minutes. Anything close to that
	// means the kill did not take, or Wait is still holding the pipes.
	if elapsed > 3*time.Second {
		t.Errorf("applyCustomRules waited %v; the timeout is meant to bound it", elapsed)
	}
}

// The syntax check is reachable from the editor on every keystroke-triggered
// validation. Unbounded, each hung call left a goroutine and a process behind
// for the life of the daemon.
func TestValidateCustomRules_TimesOutInsteadOfHanging(t *testing.T) {
	hangingNft(t, 200*time.Millisecond)

	start := time.Now()
	errs := validateCustomRules([]string{"tcp dport 9999 accept"})
	elapsed := time.Since(start)

	if len(errs) != 1 {
		t.Fatalf("expected one reported failure, got %v", errs)
	}
	if !strings.Contains(errs[0], "timed out") {
		t.Errorf("the message should say it timed out, got: %q", errs[0])
	}
	if elapsed > 3*time.Second {
		t.Errorf("validateCustomRules waited %v; the timeout is meant to bound it", elapsed)
	}
}

// Comments and blanks must not be handed to nft at all, timeout or no timeout.
func TestValidateCustomRules_SkipsCommentsWithoutRunningNft(t *testing.T) {
	hangingNft(t, 200*time.Millisecond)

	start := time.Now()
	errs := validateCustomRules([]string{"# a note", "", "   "})
	if len(errs) != 0 {
		t.Errorf("comments and blanks are not rules to check: %v", errs)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("nothing should have been executed, but it took %v", elapsed)
	}
}
