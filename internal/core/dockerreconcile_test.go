package core

import (
	"net"
	"strings"
	"testing"
	"time"
)

// fakeBridges makes detectDockerBridges return the given interfaces, one address
// each, for the duration of the test.
func fakeBridges(t *testing.T, names map[string]string) {
	t.Helper()
	origIfaces, origAddrs := netInterfacesFn, ifaceAddrsFn
	t.Cleanup(func() { netInterfacesFn, ifaceAddrsFn = origIfaces, origAddrs })

	netInterfacesFn = func() ([]net.Interface, error) {
		var out []net.Interface
		i := 1
		for name := range names {
			out = append(out, net.Interface{Index: i, Name: name})
			i++
		}
		return out, nil
	}
	ifaceAddrsFn = func(iface net.Interface) ([]net.Addr, error) {
		cidr, ok := names[iface.Name]
		if !ok {
			return nil, nil
		}
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		return []net.Addr{n}, nil
	}
}

// With Docker coexistence off, this must not run at all — no waiting, no second
// restore, on the overwhelming majority of installations.
func TestReconcileDockerBridges_DoesNothingWhenDockerIsOff(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Docker.Enabled = false
	fw := newTestFirewall(t, cfg)
	fakeBridges(t, map[string]string{"docker0": "172.17.0.1/16"})

	done := make(chan struct{})
	go func() { defer close(done); fw.reconcileDockerBridges(make(chan struct{})) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("it must return immediately when docker coexistence is off")
	}
	if got := auditActions(t, cfg); len(got) != 0 {
		t.Errorf("no restore should have been attempted, got %v", got)
	}
}

// Bridges already present at boot: nothing to wait for, nothing to redo.
func TestReconcileDockerBridges_DoesNothingWhenBridgesWereAlreadyThere(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Docker.Enabled = true
	cfg.Docker.AllowBridgeNetworks = true
	fw := newTestFirewall(t, cfg)
	fakeBridges(t, map[string]string{"docker0": "172.17.0.0/16"})

	fw.bootBridges = []string{"172.17.0.0/16"}
	// If the bootBridges check did not fire, the zero-value reconcilePoll and
	// reconcileWait left by newTestFirewall would make the next line panic
	// (time.NewTicker rejects a non-positive interval) rather than fail this
	// test cleanly. Setting both to an hour turns that failure mode into the
	// one this test already asserts on: the goroutine blocks past the 2s
	// timeout below and t.Fatal reports it, instead of the whole test binary
	// crashing on an unrelated stack trace.
	fw.reconcilePoll = time.Hour
	fw.reconcileWait = time.Hour

	done := make(chan struct{})
	go func() { defer close(done); fw.reconcileDockerBridges(make(chan struct{})) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("it must return immediately when the bridge set has not changed")
	}
	if got := auditActions(t, cfg); len(got) != 0 {
		t.Errorf("no restore should have been attempted, got %v", got)
	}
}

// The case this exists for: at boot there were no bridges, and one appears.
func TestReconcileDockerBridges_RestoresOnceWhenABridgeAppears(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Docker.Enabled = true
	cfg.Docker.AllowBridgeNetworks = true
	fw := newTestFirewall(t, cfg)
	fw.bootBridges = nil // boot saw nothing
	fw.reconcilePoll = 10 * time.Millisecond
	fw.reconcileWait = 2 * time.Second

	fakeBridges(t, map[string]string{"docker0": "172.17.0.0/16"})

	fw.reconcileDockerBridges(make(chan struct{}))

	got := auditActions(t, cfg)
	if len(got) != 1 || got[0] != "boot_enforce_failed" {
		t.Errorf("want exactly one restore attempt after the bridge appeared, got %v", got)
	}
}

// It has to give up rather than poll for ever, and say so once.
func TestReconcileDockerBridges_GivesUpAfterTheDeadline(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Docker.Enabled = true
	cfg.Docker.AllowBridgeNetworks = true
	fw := newTestFirewall(t, cfg)
	fw.bootBridges = nil
	fw.reconcilePoll = 10 * time.Millisecond
	fw.reconcileWait = 100 * time.Millisecond

	fakeBridges(t, map[string]string{"eth0": "192.0.2.1/24"}) // not a bridge

	start := time.Now()
	fw.reconcileDockerBridges(make(chan struct{}))
	elapsed := time.Since(start)

	// Both bounds matter, and neither is optional. The lower bound pins which
	// branch actually returned: only the deadline case waits out reconcileWait
	// before it does, so a bug that made this return early — wrongly, for any
	// reason — would finish in near-zero time and slip straight past a test
	// that checked only the upper bound, which is exactly what the previous
	// version of this test did. The upper bound stays generous, rather than
	// tight against 100ms, so a loaded machine's scheduler cannot turn this
	// into a flake; it is here to catch "never returns", not to time the
	// poller.
	if elapsed < fw.reconcileWait {
		t.Errorf("it returned after %v, before its own %v deadline; something "+
			"other than the deadline made it return", elapsed, fw.reconcileWait)
	}
	if elapsed > time.Second {
		t.Errorf("it waited %v; the deadline was %v", elapsed, fw.reconcileWait)
	}
	if got := auditActions(t, cfg); len(got) != 0 {
		t.Errorf("no bridge appeared, so no restore should have run, got %v", got)
	}
}

// Shutdown must not wait for the deadline.
func TestReconcileDockerBridges_StopsOnQuit(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Docker.Enabled = true
	cfg.Docker.AllowBridgeNetworks = true
	fw := newTestFirewall(t, cfg)
	fw.bootBridges = nil
	fw.reconcilePoll = 10 * time.Millisecond
	fw.reconcileWait = time.Hour

	fakeBridges(t, map[string]string{"eth0": "192.0.2.1/24"})

	quit := make(chan struct{})
	done := make(chan struct{})
	go func() { defer close(done); fw.reconcileDockerBridges(quit) }()

	time.Sleep(30 * time.Millisecond)
	close(quit)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("closing quit must end the wait")
	}
}

// A restore that happens because a Docker bridge turned up ten minutes after
// boot should not tell the operator it happened at "daemon start".
//
// The reason reaches the audit entry's detail rather than its action, so a third
// constant costs nothing anywhere else: no new label, no new colour, no
// translation.
func TestReconcileDockerBridges_UsesItsOwnRestoreReason(t *testing.T) {
	if RestoreReasonDockerBridge == RestoreReasonBoot {
		t.Fatal("RestoreReasonDockerBridge is the boot reason; the constant exists to " +
			"distinguish the two")
	}
	src := coreSource(t, "dockerreconcile.go")
	if strings.Contains(src, "RestoreReasonBoot") {
		t.Error("the reconciler still restores with RestoreReasonBoot, so a restore minutes " +
			"after boot is logged with the detail \"daemon start\"")
	}
	if !strings.Contains(src, "RestoreReasonDockerBridge") {
		t.Error("the reconciler does not use RestoreReasonDockerBridge")
	}
}

// Under panic mode the reconciler used to poll for ninety seconds and, on
// finding a bridge, log "putting the rules back" immediately before
// RestoreCurrent logged its refusal. Two adjacent lines, one of them false.
//
// This machine is deliberately unfiltered; there is nothing to reconcile, and
// nothing to say about it.
func TestReconcileDockerBridges_DoesNothingUnderPanicMode(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Docker.Enabled = true
	cfg.Docker.AllowBridgeNetworks = true
	f := newTestFirewall(t, cfg)
	f.reconcilePoll = 5 * time.Millisecond
	f.reconcileWait = 50 * time.Millisecond

	if err := EngagePanic(cfg.PanicMarkerPath()); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		f.reconcileDockerBridges(make(chan struct{}))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Millisecond):
		t.Fatal("the reconciler is still polling under panic mode; it should return at once, " +
			"before it can log \"putting the rules back\" about a machine somebody " +
			"deliberately unfiltered")
	}
}
