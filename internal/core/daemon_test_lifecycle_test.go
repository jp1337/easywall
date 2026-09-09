package core

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Running the real Daemon.Start from a test takes something that is easy to
// leave out, and four tests left it out at once on 2026-08-30. So the rule is
// not "remember it" but "there is one place that has it": startDaemonGoroutine.
// This guard says nothing else may spawn Start.
//
// The thing is observing the goroutine return. Stop closes the listener and
// waits on d.wg — which covers the boot restore, the bridge reconciler and each
// served connection, but deliberately not the accept loop (see the comment on
// Start). So Stop can return while the Start goroutine is still coming back from
// Accept, taking the quit branch and returning. The gap was measured at 3 runs in
// 200. A test that ends inside it leaves a goroutine touching t.TempDir()-scoped
// state while testing tears the frame down, which is what -race reported on main
// on 2026-08-30. Four tests carried `go func() { _ = d.Start() }()` at once,
// which discards the only signal that goroutine ever gives, so none of them
// could wait and none of them did.
//
// The helper also used to hold a slot in d.wg for as long as Start ran, so that
// no Add inside the daemon was ever the transition from zero — the flake CI hit
// on 2.15, on TestDaemonStart_RecordsAMarkerItCannotRead, reproduced at 7
// process runs in 160. That was a workaround for a hazard the daemon carried in
// production too, and it hid it: no test could see the accept loop Add at zero
// while the slot existed. The daemon now refuses that Add under d.mu instead
// (Daemon.track), the slot is gone, and
// TestDaemonStart_StopRefusesAConnectionItCannotWaitFor is what would go red if
// the refusal were removed — 76 process runs in 160 before the fix, 0 in 256
// after.
//
// Twelve tests in the default build spawn Start, four and eight: four take the
// helper's cleanup through startTestDaemon, and eight hold the channel it
// returns. Four of those eight assert on Start's own return value; the other
// four are 2.17's sd_notify tests, which hold it for a different reason —
// startTestDaemon waits for the socket, and the whole claim
// TestDaemonStart_NotifiesReadyOnlyOnceTheSocketExists makes is that the socket
// is already there when READY=1 lands, which a helper that waited for it first
// would have proven for the daemon. A thirteenth,
// TestIntegration_NewDaemon_Start_Stop, sits behind the integration tag — the
// guard reads sources rather than building them, so it is covered here whether
// that tag is set or not.
//
// The guard reads the package's own test sources, the idiom
// TestDaemonStart_SourceRestoresBeforeItListens already uses here. A pattern that
// silently matches nothing must fail, not pass — hence the count check below.
func TestDaemonTests_StartIsOnlySpawnedByTheHelper(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatalf("glob test sources: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no *_test.go found in the package: this guard is reading nothing and " +
			"would pass however the tests are written")
	}

	// A goroutine literal that runs the real Start, in any spacing and whatever
	// it does with the return value. [^{}]* keeps the match inside the
	// goroutine's own body rather than running on into the rest of a function.
	spawn := regexp.MustCompile(`go\s+func\([^)]*\)\s*\{[^{}]*d\.Start\(\)`)

	// The one function allowed to match, and the one file that must not be read:
	// this one, whose regex above would otherwise flag it forever.
	const allowed = "startDaemonGoroutine"
	const self = "daemon_test_lifecycle_test.go"

	var offenders []string
	permitted, scanned := 0, 0
	for _, f := range files {
		if f == self {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		scanned++
		// Split on top-level declarations so a match can be named after the
		// function that carries it.
		for _, chunk := range strings.Split(string(src), "\nfunc ") {
			if !spawn.MatchString(chunk) {
				continue
			}
			name := chunk
			if i := strings.IndexAny(name, "(\n"); i >= 0 {
				name = name[:i]
			}
			if name == allowed {
				permitted++
				continue
			}
			offenders = append(offenders, f+": "+name)
		}
	}

	if scanned == 0 {
		t.Fatalf("every test file was skipped as %s: the guard read nothing", self)
	}
	if permitted != 1 {
		t.Errorf("the pattern matched %s %d times, want exactly 1 — the guard is no "+
			"longer reading the one place that is allowed to spawn Start, so it would "+
			"pass however the other tests are written", allowed, permitted)
	}
	if len(offenders) > 0 {
		t.Errorf("Daemon.Start is spawned outside %s in %d place(s):\n  %s\n\n"+
			"Use it instead: it hands back the channel Start's error lands on, which "+
			"is the only signal that goroutine ever gives, and four tests discarded "+
			"it at once on 2026-08-30 — read the comment above this test.",
			allowed, len(offenders), strings.Join(offenders, "\n  "))
	}
}
