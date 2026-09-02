package core

import (
	"regexp"
	"strings"
	"testing"
)

// Acceptance.Start's own contract says it "must be called before nftables rules
// are applied", and the one call site called it after — with a panic-marker
// stat, recordAppliedConfig (a file write), a settings read and an audit write
// (a second file write) in between. On the hardware this product documents
// itself for, a Pi on an SD card, that is not microseconds. For its whole
// length the kernel held unconfirmed rules while Acceptance.Status() reported
// idle, so FirewallStatus.Acceptance said "idle" and the interface showed no
// window at all — during the one gap where being locked out is both possible
// and invisible.
//
// Held by the order of two statements and nothing else, so it is read off the
// source the way TestDaemonStart_SourceRestoresBeforeItListens reads Start.
func TestFirewallApply_OpensTheWindowBeforeItWritesTheKernel(t *testing.T) {
	body := firewallApplyBody(t)

	start := regexp.MustCompile(`f\.acceptance\.Start\(`)
	apply := regexp.MustCompile(`f\.nft\.Apply\(`)

	startAt := start.FindAllStringIndex(body, -1)
	applyAt := apply.FindAllStringIndex(body, -1)

	// A pattern that silently matches nothing has stopped guarding anything and
	// has to say so rather than pass.
	if len(startAt) != 1 {
		t.Fatalf("want exactly one f.acceptance.Start call in Firewall.apply, found %d; "+
			"this guard compares positions and cannot tell which one opens the window",
			len(startAt))
	}
	if len(applyAt) != 1 {
		t.Fatalf("want exactly one f.nft.Apply call in Firewall.apply, found %d; "+
			"this guard compares positions and cannot tell which one writes the kernel",
			len(applyAt))
	}

	if startAt[0][0] > applyAt[0][0] {
		t.Error("Firewall.apply writes the kernel before it opens the acceptance window. " +
			"Between the two run a panic-marker stat, an applied-config write, a settings " +
			"read and an audit write; for all of it the kernel holds unconfirmed rules " +
			"while Status() reports idle and the interface shows no window at all. " +
			"Acceptance.Start's own doc comment has always required this order.")
	}

	// And it has to be called, not launched. A `go func(){ f.acceptance.Start(…) }()`
	// placed above f.nft.Apply satisfies the ordering above while opening the window
	// asynchronously — the apply goroutine carries on into the kernel write without
	// waiting for Start to run at all, which destroys the guarantee the ordering
	// check above exists for.
	const asynchronous = "so nft.Apply can run before the window is actually open. It must " +
		"be a synchronous call in Firewall.apply"
	switch {
	case inGoroutine(body, startAt[0][0]):
		t.Error("f.acceptance.Start is inside a `go func` literal, " + asynchronous)
	case launchedByABareGo(body, startAt[0][0]):
		t.Error("f.acceptance.Start is launched by a bare `go` statement, " + asynchronous)
	}
}

func firewallApplyBody(t *testing.T) string {
	t.Helper()
	body := funcBody(t, coreSource(t, "firewall.go"), "firewall.go",
		"func (f *Firewall) apply(user string) error {")
	if !strings.Contains(body, "f.rules.PromoteStaged()") {
		t.Fatalf("the extracted Firewall.apply body does not promote the staged rules, so it "+
			"is not the function this test means to read:\n%s", body)
	}
	return body
}
