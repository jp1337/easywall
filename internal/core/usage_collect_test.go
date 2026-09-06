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
