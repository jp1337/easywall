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
