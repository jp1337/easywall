package core

import (
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// The socket is root:easywall 0660, so easywall-web — a non-root member of that
// group — can send anything on it. PANIC and RESUME are the two commands it
// must not be able to send: the panic banner has no control in the interface on
// purpose (docs-tech/threat-model.md), and a socket that took them from any
// client made that refusal cosmetic. This test connects as whoever runs it,
// which in CI's unit job is a non-root user standing in for easywall-web.
func TestSocket_PanicAndResumeAreRefusedFromANonRootPeer(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: the peer would be root and rightly accepted")
	}
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	configureTestFirewall(t, fw)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}
	defer d.Stop()
	startTestSocket(t, d)

	resp := sendDaemonCmd(t, cfg.SocketPath, shared.Command{Type: shared.CmdPanic})
	if resp.Success || !strings.Contains(resp.Error, "root") {
		t.Errorf("PANIC from uid %d: want a refusal naming root, got %+v", os.Geteuid(), resp)
	}
	if PanicEngaged(cfg.PanicMarkerPath()) {
		t.Fatal("PANIC from a non-root peer engaged panic mode")
	}

	if err := EngagePanic(cfg.PanicMarkerPath()); err != nil {
		t.Fatal(err)
	}
	resp = sendDaemonCmd(t, cfg.SocketPath, shared.Command{Type: shared.CmdResume})
	if resp.Success || !strings.Contains(resp.Error, "root") {
		t.Errorf("RESUME from uid %d: want a refusal naming root, got %+v", os.Geteuid(), resp)
	}
	if !PanicEngaged(cfg.PanicMarkerPath()) {
		t.Fatal("RESUME from a non-root peer cleared panic mode")
	}

	// Every other command is still the web process's to send.
	if resp := sendDaemonCmd(t, cfg.SocketPath, shared.Command{Type: shared.CmdGetStatus}); !resp.Success {
		t.Errorf("GET_STATUS from a non-root peer: %s", resp.Error)
	}
}

// The other half, which a non-root test run cannot drive end to end: the uid
// the check compares against 0 is the connecting process's, read from the
// kernel, so the root console's `easywall-core panic` is the one that passes.
func TestPeerUID_IsTheConnectingProcess(t *testing.T) {
	dir := t.TempDir()
	ln, err := net.Listen("unix", dir+"/s.sock")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		got <- c
	}()
	client, err := net.DialTimeout("unix", dir+"/s.sock", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server := <-got
	defer server.Close()

	uid, pid, err := peerCred(server)
	if err != nil {
		t.Fatalf("peerCred: %v", err)
	}
	if uid != uint32(os.Geteuid()) || pid != int32(os.Getpid()) {
		t.Errorf("peerCred = uid %d pid %d, want uid %d pid %d", uid, pid, os.Geteuid(), os.Getpid())
	}

	// Not a Unix socket: no credentials, and that must read as an error rather
	// than as uid 0.
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if _, _, err := peerCred(a); err == nil {
		t.Error("peerCred on a pipe must fail, not report a uid")
	}
}
