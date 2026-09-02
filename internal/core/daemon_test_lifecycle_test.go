package core

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A test that runs Daemon.Start in a goroutine has to be able to observe that
// goroutine returning. Calling Stop is not enough.
//
// Stop closes the listener and waits on d.wg — which covers the boot restore, the
// bridge reconciler and each served connection, but deliberately not the accept
// loop (see the comment on Start). So Stop can return while the Start goroutine
// is still coming back from Accept, taking the quit branch and returning. The gap
// was measured at 3 runs in 200. A test that ends inside that gap leaves a
// goroutine touching t.TempDir()-scoped state while testing tears the frame down,
// which is what -race reported on main on 2026-08-30 as a race between
// (*Daemon).Stop-fm running from t.Cleanup and the goroutine at daemon_test.go:772.
//
// What this guard forbids is the one spelling that throws the signal away:
//
//	go func() { _ = d.Start() }()
//
// Discarding the return value leaves nothing to wait on, which is precisely why
// four tests carrying it could not wait and did not. The two spellings that keep
// it are fine and are not flagged: `errCh <- d.Start()` with a receive on errCh —
// four tests do that and always did — and startTestDaemon, which closes a done
// channel and blocks on it in its cleanup.
//
// The guard reads the package's own test sources, the idiom
// TestDaemonStart_SourceRestoresBeforeItListens already uses here. A pattern that
// silently matches nothing must fail, not pass — hence the file check below.
func TestDaemonTests_StartInAGoroutineIsAlwaysWaitedFor(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatalf("glob test sources: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no *_test.go found in the package: this guard is reading nothing and " +
			"would pass however the tests are written")
	}

	// The discarded-return spelling, in any spacing.
	discard := regexp.MustCompile(`go\s+func\(\)\s*\{\s*_\s*=\s*d\.Start\(\)`)

	// Reading its own regex back out of this file would flag it forever.
	self := "daemon_test_lifecycle_test.go"

	var offenders []string
	scanned := 0
	for _, f := range files {
		if f == self {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		scanned++
		for _, m := range discard.FindAllString(string(src), -1) {
			offenders = append(offenders, f+": "+strings.Join(strings.Fields(m), " ")+" }()")
		}
	}

	if scanned == 0 {
		t.Fatalf("every test file was skipped as %s: the guard read nothing", self)
	}

	if len(offenders) > 0 {
		t.Errorf("Daemon.Start is spawned with its return discarded in %d place(s):\n  %s\n\n"+
			"Use startTestDaemon(t, d), which waits for Start to return in its cleanup. "+
			"Stop does not wait for the accept loop, so a goroutine started this way "+
			"outlives the test and races testing's own teardown.",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}
