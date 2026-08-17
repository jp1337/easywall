package core

import (
	"net"
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

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("it waited %v; the deadline was 100ms", elapsed)
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
