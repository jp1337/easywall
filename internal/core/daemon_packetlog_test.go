package core

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// Review Focus 2. A bind that fails must leave the log rules writing to the
// kernel log — an apply that failed because the packet log was unavailable
// would stop a firewall over a viewer.
func TestStartPacketLog_ABindFailureKeepsTheKernelLog(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{}), packets: NewPacketLog(10)}
	d.listenPacketLog = func(*PacketLog, uint16) (func(), error) { return nil, errors.New("device or resource busy") }

	d.startPacketLog()

	if fw.nft.logSink.nflog {
		t.Error("the sink moved to NFLOG with nobody listening; every logged packet would vanish")
	}
	res, _ := d.packets.Query(shared.PacketLogFilter{})
	if res.Listening || res.Reason == "" {
		t.Errorf("the reply says listening=%v reason=%q; the page has to be able to say why", res.Listening, res.Reason)
	}
	if res.Stopped {
		t.Error("a bind that never succeeded reports Stopped=true; the kernel-log fallback line applies here, the restart line does not")
	}
}

// Final review, Important 2. A bind failure and a listener that ran and then
// died read the same (Listening=false) but must not tell the operator the
// same thing: a bind failure still has the kernel-log fallback; a listener
// that stops after running does not, because the log rules still name the now
// unread NFLOG group. Stopped is what lets /blocked tell them apart.
func TestPacketLog_OnErrorAfterListeningLeavesNothingLoggedAnywhere(t *testing.T) {
	p := NewPacketLog(10)
	p.setListening(12227, nil) // the bind succeeded once

	if code := p.onError(errors.New("netlink receive: connection reset by peer")); code != 1 {
		t.Fatalf("onError returned %d, want 1 (stop reading on a non-ENOBUFS error)", code)
	}

	res, _ := p.Query(shared.PacketLogFilter{})
	if res.Listening {
		t.Error("still Listening after onError stopped the receive loop")
	}
	if !res.Stopped {
		t.Error("Stopped is false; a listener that ran and died is indistinguishable from a bind that never succeeded")
	}
}

func TestPacketLog_ABindThatNeverSucceededIsNotStopped(t *testing.T) {
	p := NewPacketLog(10)
	p.setListening(12227, errors.New("device or resource busy"))

	res, _ := p.Query(shared.PacketLogFilter{})
	if res.Stopped {
		t.Error("Stopped is true on a bind that never succeeded; the kernel-log fallback line no longer applies for it")
	}
}

func TestStartPacketLog_ABindPointsTheRulesAtTheGroup(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{}), packets: NewPacketLog(10)}
	d.listenPacketLog = func(p *PacketLog, g uint16) (func(), error) { p.setListening(g, nil); return func() {}, nil }
	defer d.Stop()

	d.startPacketLog()

	if !fw.nft.logSink.nflog || fw.nft.logSink.group != cfg.PacketLogGroup() {
		t.Errorf("sink = %+v, want NFLOG group %d", fw.nft.logSink, cfg.PacketLogGroup())
	}
}

func TestGetPacketLogIsAnsweredFromTheRing(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{}), packets: NewPacketLog(10)}
	d.packets.Add(shared.PacketLogEntry{Rule: "drop", Proto: "tcp", DstPort: 22})

	resp := d.dispatch(shared.Command{Type: shared.CmdGetPacketLog, Payload: []byte(`{"port":22}`)})
	var res shared.PacketLogResult
	if !resp.Success || json.Unmarshal(resp.Data, &res) != nil || res.Matched != 1 {
		t.Fatalf("resp %+v, result %+v", resp, res)
	}

	resp = d.dispatch(shared.Command{Type: shared.CmdGetPacketLog, Payload: []byte(`{"src":"nope"}`)})
	if resp.Success {
		t.Error("the core answered a filter it cannot interpret")
	}
	// No payload at all is a request for everything, not a JSON error.
	if resp := d.dispatch(shared.Command{Type: shared.CmdGetPacketLog}); !resp.Success {
		t.Errorf("an empty payload was refused: %s", resp.Error)
	}
}

// A Stop that lands while the bind is in flight must still end the listener:
// Stop reads packetsStop once, and a stop func installed after that read would
// leave the NFLOG group bound by a daemon that has already shut down.
func TestStartPacketLog_AStopDuringTheBindEndsTheListener(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{}), packets: NewPacketLog(10)}
	running := false
	d.listenPacketLog = func(p *PacketLog, g uint16) (func(), error) {
		running = true
		d.Stop() // SIGTERM arrives while the bind is being made
		return func() { running = false }, nil
	}

	d.startPacketLog()

	if running {
		t.Error("the listener is still running after Stop returned")
	}
}
