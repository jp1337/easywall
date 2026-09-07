package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Panic writes the marker and then tears the table down; Resume clears the
// marker and then restores. Interleaved, ClearPanic can land between
// EngagePanic and nft.Reset — and the machine ends with no marker and an empty
// table: unfiltered, and nothing anywhere recording that anyone chose it. The
// dashboard goes red and `easywall-core status` exits 2, so it is visible; it is
// also three lines of mutex to make impossible — provided the lock is taken
// before either function touches the marker at all. A lock acquired after
// EngagePanic/ClearPanic has already run reopens exactly that window while
// still satisfying "the function mentions panicMu somewhere", which is why
// this checks the lock's *position*, not merely its presence: it asserts
// f.panicMu.Lock() is the first statement in each body and that the deferred
// unlock immediately follows it, rather than searching the whole body text for
// the two substrings.
//
// Source-read, because the interleaving is a window of a few statements between
// two console commands and no runtime test can open it reliably — this proves
// the mutual-exclusion mechanism is wired up correctly on both paths, not that
// the race has been exercised. This is the idiom daemon_source_order_test.go
// already uses for a guarantee a runtime test cannot reach.
func TestPanicAndResumeShareALock(t *testing.T) {
	src := coreSource(t, "restore.go")

	if !strings.Contains(coreSource(t, "firewall.go"), "panicMu sync.Mutex") {
		t.Fatal("Firewall declares no panicMu; the assertions below are about a field " +
			"that does not exist")
	}

	const lockLine = "f.panicMu.Lock()"
	const deferLine = "defer f.panicMu.Unlock()"

	for _, sig := range []string{
		"func (f *Firewall) Panic(",
		"func (f *Firewall) Resume(",
	} {
		body := funcBody(t, src, "restore.go", sig)

		// Leading whitespace only (indentation, blank lines) is trimmed — any
		// other statement ahead of the lock, including EngagePanic/ClearPanic,
		// fails this.
		lead := strings.TrimLeft(body, " \t\n")
		if !strings.HasPrefix(lead, lockLine) {
			t.Errorf("%s does not take panicMu as its first statement; a lock acquired "+
				"after the marker has already been written or cleared leaves the same "+
				"window open the lock exists to close", sig)
			continue
		}

		afterLock := strings.TrimLeft(strings.TrimPrefix(lead, lockLine), " \t\n")
		if !strings.HasPrefix(afterLock, deferLine) {
			t.Errorf("%s does not defer-unlock panicMu immediately after taking it; an "+
				"early return before a plain Unlock() wedges every later panic and resume",
				sig)
		}
	}
}

// The helper is told what the marker said; it does not go and look again.
//
// The rollback site stats the marker three times — twice in its own gate, once
// more inside the helper — and a marker that becomes unreadable between the
// second read and the third reintroduces the inversion the gate exists to
// prevent: PanicState says "not engaged, and that is known", and PanicEngaged a
// microsecond later says "engaged" and tears the rules down.
func TestPanicLandedDuringWriteIsToldTheMarkerState(t *testing.T) {
	body := funcBody(t, coreSource(t, "restore.go"), "restore.go",
		"func (f *Firewall) panicLandedDuringWrite(")
	for _, reread := range []string{"f.PanicEngaged()", "PanicState("} {
		if strings.Contains(body, reread) {
			t.Errorf("panicLandedDuringWrite calls %s; it has to act on the state its caller "+
				"already read, or the marker is stat'd again and can disagree with the gate",
				reread)
		}
	}
}

// And behaviourally: told the marker is absent, it does nothing, even with a
// marker sitting on disk.
func TestPanicLandedDuringWrite_ActsOnWhatItIsTold(t *testing.T) {
	cfg := newTestConfig(t)
	f := newTestFirewall(t, cfg)

	if err := os.WriteFile(filepath.Join(cfg.DataDir, "panic"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if f.panicLandedDuringWrite(false, "apply_refused_panic", "told the marker was absent", "test") {
		t.Error("panicLandedDuringWrite tore the table down after being told panic mode was " +
			"not engaged; it is re-reading the marker rather than acting on its argument")
	}
}
