package core

import (
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// An idle client — dialled, never written to, never closed — used to hold
// Stop for as long as it liked: handleConn sat in io.ReadAll with no deadline
// and its d.wg slot kept Stop's Wait open, so `systemctl restart
// easywall-core` waited out TimeoutStopSec and ended in SIGKILL.
func TestDaemonStop_DoesNotWaitForAnIdleClient(t *testing.T) {
	cfg := newTestConfig(t)
	off := 0
	cfg.Usage.Interval = &off
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}
	errCh := startDaemonGoroutine(d)
	waitForSocket(t, cfg.SocketPath)

	idle, err := net.Dial("unix", cfg.SocketPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer idle.Close()
	time.Sleep(100 * time.Millisecond) // let the accept loop hand it to handleConn

	stopped := make(chan struct{})
	start := time.Now()
	go func() { d.Stop(); close(stopped) }()
	select {
	case <-stopped:
		t.Logf("Stop returned after %v with an idle client connected", time.Since(start))
	case <-time.After(3 * time.Second):
		t.Fatalf("Stop still blocked after 3s on an idle client")
	}
	<-errCh
}

// The read deadline bounds a client that never finishes its request, and it is
// long enough for every command's own client deadline: a client that waits
// longer than the core reads would be cut off while it is still within budget.
func TestDaemonConnTimeout_CoversEveryClientDeadline(t *testing.T) {
	for _, c := range shared.AllCommandTypes {
		if got := shared.CommandTimeout(c); got > connTimeout {
			t.Errorf("%s: the client waits %v, the core only %v", c, got, connTimeout)
		}
	}
}

// servedConns counts the connections waiting on their peer — every served one
// in the tests below, which never send a request.
func (d *Daemon) servedConns() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.peerWait)
}

// More clients than maxConns at once are queued, not served: each served one
// can hold up to MaxMessageBytes of request in memory, and 32 clients each
// sending 8 MiB−1 without closing took the heap from 4 MiB to 286 MiB.
func TestDaemon_ServesAtMostMaxConnsAtOnce(t *testing.T) {
	cfg := newTestConfig(t)
	off := 0
	cfg.Usage.Interval = &off
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}
	startTestDaemon(t, d)

	var held []net.Conn
	defer func() {
		for _, c := range held {
			_ = c.Close()
		}
	}()
	for range maxConns + 4 {
		c, err := net.Dial("unix", cfg.SocketPath)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		held = append(held, c)
	}
	time.Sleep(200 * time.Millisecond)
	if got := d.servedConns(); got != maxConns {
		t.Errorf("serving %d connections at once, want the cap %d", got, maxConns)
	}

	// A slot freed is a queued client served: close one idle client and the
	// next in line is taken.
	_ = held[0].Close()
	held = held[1:]
	deadline := time.Now().Add(2 * time.Second)
	for d.servedConns() != maxConns && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := d.servedConns(); got != maxConns {
		t.Errorf("after one closed: serving %d, want %d", got, maxConns)
	}
}

// The console is never queued behind the web process. The cap used to be a
// semaphore in front of Accept, so maxConns idle connections from the
// unprivileged side left `easywall-core panic` in the listen backlog until its
// client gave up. A test has no root peer of its own (or, under sudo, only
// root peers), so the console here is the connection dialled from a named
// address.
func TestDaemon_TheConsoleIsServedWhileTheCapIsFull(t *testing.T) {
	cfg := newTestConfig(t)
	off := 0
	cfg.Usage.Interval = &off
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	consoleAddr := filepath.Join(t.TempDir(), "console.sock")
	saved := isConsolePeer
	isConsolePeer = func(conn net.Conn) bool { return conn.RemoteAddr().String() == consoleAddr }
	t.Cleanup(func() { isConsolePeer = saved })
	startTestDaemon(t, d)

	var held []net.Conn
	defer func() {
		for _, c := range held {
			_ = c.Close()
		}
	}()
	for range maxConns + maxQueued + 4 {
		c, err := net.Dial("unix", cfg.SocketPath)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		held = append(held, c)
	}

	console, err := net.DialUnix("unix", &net.UnixAddr{Name: consoleAddr, Net: "unix"},
		&net.UnixAddr{Name: cfg.SocketPath, Net: "unix"})
	if err != nil {
		t.Fatalf("dial as the console: %v", err)
	}
	defer console.Close()
	req, _ := json.Marshal(shared.Command{Type: shared.CmdGetStatus})
	if _, err := console.Write(req); err != nil {
		t.Fatal(err)
	}
	_ = console.CloseWrite()
	_ = console.SetReadDeadline(time.Now().Add(3 * time.Second))
	var resp shared.Response
	if err := json.NewDecoder(console).Decode(&resp); err != nil {
		t.Fatalf("the console was not served while the web side held every slot: %v", err)
	}
	if !resp.Success {
		t.Errorf("console request failed: %s", resp.Error)
	}
}
