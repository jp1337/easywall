package core

import (
	"encoding/json"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The guard that refuses a second apply, tested without a kernel.
//
// The kernel test for this (TestIntegration_SecondApplyIsRefusedNotQueued) proves
// the *consequence* — that a queued apply re-applies rules the first one rolled
// back, and that shutdown waits a full window per queued request. It needs an
// apply that actually reaches a kernel, so it only runs under -tags integration
// and a plain `go test ./internal/...` never sees it.
//
// What is tested here is the mechanism: an apply cycle in progress means the next
// APPLY_RULES is answered with a refusal rather than "started", and the slot is
// released afterwards. beginApply/endApply bracket the cycle, so the state can be
// set up directly and no rule has to reach anything.
func TestDispatchRefusesApplyWhileACycleIsRunning(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	// Stand in for a cycle that is running — which, in production, is a cycle
	// sitting in its acceptance window for up to an hour.
	if !fw.beginApply() {
		t.Fatal("beginApply refused on an idle firewall")
	}

	resp := d.dispatch(shared.Command{Type: shared.CmdApplyRules})
	if resp.Success {
		var data map[string]string
		_ = json.Unmarshal(resp.Data, &data)
		t.Errorf("APPLY during a running cycle answered success=%v data=%v; it used to answer "+
			`"started" and queue the work behind the open acceptance window`, resp.Success, data)
	}
	if resp.Error != shared.ErrApplyInProgressText {
		t.Errorf("Response.Error = %q, want %q — the web process matches on this exact string "+
			"to say \"an apply is already running\" instead of reporting a generic failure",
			resp.Error, shared.ErrApplyInProgressText)
	}

	// Direct callers are refused too, rather than blocking until the window ends.
	if err := fw.Apply("probe"); err != ErrApplyInProgress {
		t.Errorf("Firewall.Apply during a running cycle returned %v, want ErrApplyInProgress", err)
	}

	// And the slot is not one-shot: once the cycle ends, an apply is allowed.
	fw.endApply()
	if !fw.beginApply() {
		t.Error("the apply slot was not released by endApply; no further apply would ever be accepted")
	}
	fw.endApply()
}

// beginApply is the whole invariant, so it is worth asserting on its own that it
// admits exactly one holder.
func TestBeginApplyAdmitsOneHolder(t *testing.T) {
	fw := newTestFirewall(t, newTestConfig(t))

	if !fw.beginApply() {
		t.Fatal("first beginApply refused")
	}
	if fw.beginApply() {
		t.Error("beginApply admitted a second holder; two apply cycles could run at once")
	}
	fw.endApply()
	if !fw.beginApply() {
		t.Error("beginApply refused after endApply")
	}
	fw.endApply()
}
