package core

import (
	"strings"
	"testing"
)

// The flush zeroes every kernel counter, so an apply that writes before it reads
// throws the whole interval since the last tick away — up to five minutes of
// traffic on the default, and on a host with the ticker switched off, everything
// since the previous apply.
//
// Source-read, and it has to be: the two calls are ordinary statements in one
// function, and no runtime assertion can see which ran first without a kernel
// and a packet generator. This is the same idiom
// TestEveryKernelWriteIsFollowedByThePanicCheck uses, for the same reason.
func TestApplyCollectsBeforeItFlushes(t *testing.T) {
	body := funcBody(t, coreSource(t, "firewall.go"), "firewall.go", "func (f *Firewall) apply(")

	collects := indexesOf(body, "f.collectUsageBeforeWrite(")
	writes := indexesOf(body, "f.nft.Apply(")
	if len(collects) != 1 {
		t.Fatalf("apply calls f.collectUsageBeforeWrite %d times, want exactly 1; the counters "+
			"have to be booked once, before the write that resets them", len(collects))
	}
	if len(writes) != 1 {
		t.Fatalf("apply calls f.nft.Apply %d times, want exactly 1; this guard compares "+
			"against a single write", len(writes))
	}
	if collects[0] > writes[0] {
		t.Error("apply writes the kernel before it reads the counters. The write flushes the " +
			"table and every counter in it goes back to zero, so the interval since the last " +
			"collect is lost — silently, and it looks exactly like an idle port")
	}
}

// And the baselines are put back to zero once the table has been rebuilt, or the
// next delta is measured against counts that are no longer in the kernel: the
// new counters have to climb past the old totals before anything is booked at
// all, which on a busy port is an hour of traffic reported as silence.
//
// After the write and not before it, deliberately. nft.Apply refuses before
// touching the table when ValidateRules fails, and a baseline zeroed at that
// point re-books the entire lifetime of every live kernel rule at the next
// collect. Spec amendment A4.
func TestARebuiltTableDoesNotLoseTheInterval(t *testing.T) {
	body := funcBody(t, coreSource(t, "firewall.go"), "firewall.go", "func (f *Firewall) apply(")

	resets := indexesOf(body, "f.resetUsageBaselines(")
	writes := indexesOf(body, "f.nft.Apply(")
	if len(resets) != 1 {
		t.Fatalf("apply calls f.resetUsageBaselines %d times, want exactly 1", len(resets))
	}
	if len(writes) != 1 {
		t.Fatalf("apply calls f.nft.Apply %d times, want exactly 1", len(writes))
	}
	if resets[0] < writes[0] {
		t.Error("apply resets the usage baselines before the kernel write. nft.Apply returns " +
			"before touching the table when validation refuses, and a baseline zeroed there " +
			"re-books the whole lifetime of every live rule at the next collect")
	}
}

// The ticker exists so "last used" has a resolution that does not depend on
// somebody opening the page, and it is switched off by an interval of zero
// rather than by deleting the goroutine.
func TestTheUsageTickerIsWiredIntoTheDaemon(t *testing.T) {
	body := daemonStartBody(t)
	if !strings.Contains(body, "collectUsagePeriodically") {
		t.Error("Daemon.Start never launches collectUsagePeriodically; the counters are only " +
			"ever read by an apply, so a port used between two applies reports as never used")
	}
}

// Every write that destroys the kernel counters books them first.
//
// This is the guard the two above could not be. Both scope themselves to
// funcBody(… "func (f *Firewall) apply(") — the one function the collect was
// written into — and four of the five kernel writes in this package were
// therefore invisible to them. The rollback case is every unconfirmed apply:
// collect at T+0, apply at T+2min, window expires at T+4min, and the flush took
// two minutes of traffic with it, so a port whose only use was in that window
// reads "never" and the dashboard advises closing it.
//
// So it borrows the mechanism from TestEveryKernelWriteIsFollowedByThePanicCheck
// instead: coreSources globs every non-test file in the package, each write site
// is enumerated with the reason for its counts, and any occurrence of either
// token that the table does not account for fails. A write added in a sixth
// place, or in a new file, cannot be added without answering to this.
//
// Two tokens count as destroying the counters:
//
//   - f.nft.Apply — deletes and recreates the table (see NftablesManager.reset),
//     so every counter in it goes back to zero.
//   - f.nft.Reset — deletes the table outright. There is nothing to reset a
//     baseline to afterwards; see panicLandedDuringWrite's comment.
//
// What it cannot see is the same short list the panic guard names: a call kept
// textually and wrapped in `if false`, and the call order beyond "before the
// write" / "after the write".
func TestEveryKernelWriteBooksTheCountersFirst(t *testing.T) {
	const collect = "f.collectUsageBeforeWrite("
	const reset = "f.resetUsageBaselines("
	writes := []string{"f.nft.Apply(", "f.nft.Reset("}

	sources := coreSources(t)

	// The two helpers have to exist, or every assertion below is about a name
	// nothing implements.
	for _, fn := range []string{
		"func (f *Firewall) collectUsageBeforeWrite(",
		"func (f *Firewall) resetUsageBaselines(",
	} {
		if !strings.Contains(sources["usage.go"], fn) {
			t.Fatalf("%s is not defined in usage.go; this guard is checking calls to a "+
				"function that no longer exists", fn)
		}
	}

	sites := []struct {
		file, sig                string
		wantCollects, wantResets int
		why                      string
	}{
		{"firewall.go", "func (f *Firewall) apply(", 1, 1,
			"the flush zeroes every counter, so the interval since the last tick is " +
				"booked before it and the baselines are put back to zero after it"},
		{"firewall.go", "func (f *Firewall) rollback(", 1, 1,
			"every unconfirmed apply ends here, and this write flushes the table just " +
				"like an apply's does — the traffic of the acceptance window is in those " +
				"counters and nowhere else"},
		{"restore.go", "func (f *Firewall) RestoreCurrent(", 1, 1,
			"a restore is not only a boot: RESUME and the Docker-bridge reconciler both " +
				"reach it on a machine that has been filtering for weeks"},
		{"restore.go", "func (f *Firewall) panicLandedDuringWrite(", 1, 0,
			"the teardown deletes the table, so the counters are booked first and there " +
				"is nothing left for a baseline to describe"},
		{"restore.go", "func (f *Firewall) Panic(", 1, 0,
			"same shape: what was in use before the operator panicked is part of what " +
				"they need in order to decide what to change"},
	}

	found := map[string]map[string]int{}
	record := func(file, token string, n int) {
		if found[file] == nil {
			found[file] = map[string]int{}
		}
		found[file][token] += n
	}

	for _, s := range sites {
		body := funcBody(t, sources[s.file], s.file, s.sig)

		var writeAt []int
		for _, w := range writes {
			at := indexesOf(body, w)
			writeAt = append(writeAt, at...)
			record(s.file, w, len(at))
		}
		if len(writeAt) != 1 {
			t.Errorf("%s: %s contains %d calls that destroy the kernel counters, want "+
				"exactly 1; this guard compares against a single write and cannot tell "+
				"which one the bookkeeping belongs to", s.file, s.sig, len(writeAt))
			continue
		}

		collects := indexesOf(body, collect)
		resets := indexesOf(body, reset)
		record(s.file, collect, len(collects))
		record(s.file, reset, len(resets))

		if len(collects) != s.wantCollects {
			t.Errorf("%s: %s calls %s %d time(s), want %d — %s. Without it the traffic "+
				"since the last collect is destroyed by the write and the port reads as "+
				"though nothing ever reached it",
				s.file, s.sig, collect, len(collects), s.wantCollects, s.why)
		}
		if len(resets) != s.wantResets {
			t.Errorf("%s: %s calls %s %d time(s), want %d — %s",
				s.file, s.sig, reset, len(resets), s.wantResets, s.why)
		}
		for _, at := range collects {
			if at > writeAt[0] {
				t.Errorf("%s: %s books the counters after the write that destroys them; "+
					"read before, or there is nothing left to read", s.file, s.sig)
			}
		}
		for _, at := range resets {
			if at < writeAt[0] {
				t.Errorf("%s: %s resets the baselines before the kernel write. nft.Apply "+
					"returns before touching the table when validation refuses, and a "+
					"baseline zeroed there re-books the whole lifetime of every live rule "+
					"at the next collect", s.file, s.sig)
			}
		}
	}

	// Nothing went unparsed. A write, a collect or a reset anywhere the table
	// above does not cover means this guard has stopped looking at every kernel
	// write — which is exactly the failure mode that let four of the five sites
	// go unaccounted for in the first place, so it fails rather than quietly
	// narrowing.
	for file, src := range sources {
		for _, token := range append(append([]string{}, writes...), collect, reset) {
			got, accounted := len(indexesOf(src, token)), found[file][token]
			if got == accounted {
				continue
			}
			t.Errorf("%s contains %d calls to %s but this guard accounted for %d; a "+
				"kernel write outside apply, rollback, RestoreCurrent, "+
				"panicLandedDuringWrite and Panic has to be added to the table in this "+
				"test together with its own collect — and a collect or reset outside "+
				"them is not being asserted about at all", file, got, token, accounted)
		}
	}
}
