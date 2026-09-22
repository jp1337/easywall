//go:build integration

package core

import (
	"errors"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// Each test below binds its own group. listen's stop closes its socket
// synchronously, so within one test rebinding after stop is always safe —
// but two tests sharing a group would still race whichever runs second
// against the first's teardown, so each gets its own instead.
const (
	testNFLOGGroupDropped = 12228
	testNFLOGGroupSecond  = 12229
)

// Spec §8, verbatim: a real packet, dropped by a real rule in a real
// namespace, appears as a typed event carrying the right prefix, source and
// port.
func TestIntegration_ADroppedPacketArrivesTyped(t *testing.T) {
	h, err := NewHarness()
	if errors.Is(err, ErrNamespaceUnavailable) {
		skipOrFailUnprovable(t, err.Error())
	}
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	p := NewPacketLog(100)
	stop, err := p.listen(testNFLOGGroupDropped)
	if err != nil {
		// On a host where nfnetlink_log cannot be autoloaded from here —
		// rootless podman — `sudo modprobe nfnetlink_log` first. CI runs
		// under sudo and must not skip.
		skipOrFailUnprovable(t, "cannot bind an NFLOG group: "+err.Error())
	}
	defer stop()

	m := newIntegrationManager(t)
	m.SetLogSink(logSink{nflog: true, group: testNFLOGGroupDropped})
	opts := shared.FirewallOptions{LogBlocked: true, LogBlockedLimit: 600}
	if err := m.Apply(emptyState(), opts, shared.NetworkSettings{IPv6: shared.IPv6Config{Mode: shared.IPv6Filter}}); err != nil {
		t.Fatalf("Apply with an NFLOG sink: %v — if EINVAL, a flag or level is set beside the group", err)
	}

	const port = 12230 // nothing is open, so the policy drops and the final rule logs
	if open, err := h.Dial(h.RouterAddr(), port, time.Second); err != nil || open {
		t.Fatalf("dial: open=%v err=%v; the drop policy should have refused it", open, err)
	}

	var got shared.PacketLogEntry
	waitUntil(t, 3*time.Second, func() bool {
		res, _ := p.Query(shared.PacketLogFilter{Port: port})
		if len(res.Entries) > 0 {
			got = res.Entries[0]
			return true
		}
		return false
	})
	if got.Rule != "drop" || got.Src != h.PeerAddr() || got.Dst != h.RouterAddr() ||
		got.Proto != "tcp" || got.DstPort != port || got.TCPFlags != "SYN" ||
		got.InDev != harnessRouterIf || got.Hook != "input" {
		t.Errorf("entry %+v; want rule drop, %s → %s:%d tcp SYN on %s, hook input",
			got, h.PeerAddr(), h.RouterAddr(), port, harnessRouterIf)
	}
}

// Review Focus 2: a group binds once per namespace. The second binder gets an
// error it can name — which is the whole difference between a collision that
// is diagnosable and one that is mysterious.
func TestIntegration_ASecondBinderIsRefusedLoudly(t *testing.T) {
	first := NewPacketLog(10)
	stop, err := first.listen(testNFLOGGroupSecond)
	if err != nil {
		skipOrFailUnprovable(t, "cannot bind an NFLOG group: "+err.Error())
	}
	defer stop()

	second := NewPacketLog(10)
	_, err = second.listen(testNFLOGGroupSecond)
	if err == nil {
		t.Fatal("a second listener bound the same group")
	}
	res, _ := second.Query(shared.PacketLogFilter{})
	if res.Listening || res.Reason == "" || res.Group != testNFLOGGroupSecond {
		t.Errorf("the refused listener reports %+v; the page must be able to name the group and why", res)
	}
}

func waitUntil(t *testing.T, within time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met within " + within.String())
}
