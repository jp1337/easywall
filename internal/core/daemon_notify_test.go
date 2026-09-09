package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// READY=1 is a claim about the socket, not about the process.
//
// The bug Type=notify closes is an ordering one: under Type=simple the unit was
// active (running) the instant exec succeeded, easywall-web.service's
// After=easywall-core.service was satisfied by that, and the socket the web
// process actually needs did not exist for another rule restore. So the thing
// worth testing is not that a datagram is sent but *when*: nothing in this test
// waits for the socket, so the socket existing at the moment the datagram
// arrives is the ordering itself.
func TestDaemonStart_NotifiesReadyOnlyOnceTheSocketExists(t *testing.T) {
	ln := listenNotify(t, filepath.Join(t.TempDir(), "notify"))
	t.Setenv(notifySocketEnv, ln.LocalAddr().String())
	// No watchdog here, so READY=1 is the only datagram this socket can see and
	// a WATCHDOG=1 cannot be mistaken for it.
	t.Setenv(watchdogUSecEnv, "")

	cfg := newTestConfig(t)
	d := &Daemon{cfg: cfg, firewall: newTestFirewall(t, cfg), quit: make(chan struct{})}
	errCh := startDaemonGoroutine(d)
	t.Cleanup(func() {
		d.Stop()
		<-errCh
	})

	if got := readNotify(t, ln); got != "READY=1" {
		t.Fatalf("datagram = %q, want READY=1", got)
	}
	if _, err := os.Stat(cfg.SocketPath); err != nil {
		t.Errorf("READY=1 arrived before %s existed (%v) — systemd would report the "+
			"core as active and start easywall-web into a socket that is not there, "+
			"which is the whole race Type=notify was added to close",
			cfg.SocketPath, err)
	}
}

// The watchdog has to actually ping, or WatchdogSec= in the unit is a 60-second
// fuse under a healthy daemon.
func TestDaemonStart_PingsTheWatchdog(t *testing.T) {
	ln := listenNotify(t, filepath.Join(t.TempDir(), "notify"))
	t.Setenv(notifySocketEnv, ln.LocalAddr().String())
	// 20ms, so the ping interval is 10ms and this test costs milliseconds
	// rather than the minute the shipped unit asks for.
	t.Setenv(watchdogUSecEnv, "20000")

	cfg := newTestConfig(t)
	d := &Daemon{cfg: cfg, firewall: newTestFirewall(t, cfg), quit: make(chan struct{})}
	errCh := startDaemonGoroutine(d)
	t.Cleanup(func() {
		d.Stop()
		<-errCh
	})

	if got := readNotify(t, ln); got != "READY=1" {
		t.Fatalf("first datagram = %q, want READY=1", got)
	}
	if got := readNotify(t, ln); got != "WATCHDOG=1" {
		t.Errorf("second datagram = %q, want WATCHDOG=1", got)
	}
}

// The failure a watchdog exists to catch is a wedged process, and a bare ticker
// cannot catch it: it goes on satisfying systemd for ever regardless of what the
// daemon is doing. So the ping is gated on d.mu — the lock Start takes to
// install the listener, Stop takes to remove it, and track takes for every piece
// of work the daemon starts — and this test holds that lock and asserts the
// pings stop.
//
// The lock is released by a defer rather than at the end of the body. A t.Fatalf
// here while holding d.mu would otherwise leave t.Cleanup calling Stop, which
// takes d.mu, and the package would hang instead of failing. This release has
// already produced one hang-instead-of-failure, and a hang in CI reads as
// nothing at all.
func TestDaemonStart_TheWatchdogStopsOnAWedgeAndResumesAfterIt(t *testing.T) {
	ln := listenNotify(t, filepath.Join(t.TempDir(), "notify"))
	t.Setenv(notifySocketEnv, ln.LocalAddr().String())
	t.Setenv(watchdogUSecEnv, "20000") // a 10ms ping interval

	cfg := newTestConfig(t)
	d := &Daemon{cfg: cfg, firewall: newTestFirewall(t, cfg), quit: make(chan struct{})}
	errCh := startDaemonGoroutine(d)
	t.Cleanup(func() {
		d.Stop()
		<-errCh
	})

	// Wait until the watchdog is demonstrably running, or a silent window below
	// would prove nothing more than that it had not started yet.
	if got := readNotify(t, ln); got != "READY=1" {
		t.Fatalf("first datagram = %q, want READY=1", got)
	}
	if got := readNotify(t, ln); got != "WATCHDOG=1" {
		t.Fatalf("second datagram = %q, want WATCHDOG=1", got)
	}

	buf := make([]byte, 64)

	// A closure so the lock is released by its own defer whichever way this
	// leaves — a t.Fatalf runs deferred functions during Goexit, and a t.Cleanup
	// that reached Stop while this test still held d.mu would hang the package
	// rather than fail it.
	func() {
		d.mu.Lock()
		defer d.mu.Unlock()

		// Drain what was sent or queued before the lock landed, plus the one
		// ping the first failed TryLock is still allowed to send — pingWatchdog
		// stops on the second consecutive failure, not the first, because a
		// single unlucky sample must not put a healthy daemon on systemd's
		// deadline. 200ms is twenty ping intervals, so that grace ping is
		// certainly inside this window.
		_ = ln.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		for {
			if _, _, err := ln.ReadFrom(buf); err != nil {
				break
			}
		}
		// From here every ping has to get the lock this test is holding. 500ms
		// is fifty ping intervals.
		_ = ln.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		if n, _, err := ln.ReadFrom(buf); err == nil {
			t.Errorf("a ping arrived while the daemon's own lock was held: %q — the "+
				"watchdog is not reading the daemon's state, so a wedged core would "+
				"keep systemd satisfied for ever, which is the one failure WatchdogSec "+
				"exists to catch", buf[:n])
		}
	}()

	// And it has to come back. The counter is consecutive failures, not a total,
	// and nothing caught that until this half existed: dropping `missed = 0`
	// from the success branch left the whole suite green while a single
	// contended sample would latch the daemon into silence for the rest of the
	// process — and systemd would then kill a perfectly healthy firewall daemon
	// on the watchdog. That is the same defect as a bare ticker with the sign
	// reversed, and it is the more expensive of the two.
	_ = ln.SetReadDeadline(time.Now().Add(2 * time.Second))
	if n, _, err := ln.ReadFrom(buf); err != nil {
		t.Errorf("no ping in two seconds after the lock was released: %v — the "+
			"failure counter is latching instead of counting consecutive misses, "+
			"so one moment of contention silences the daemon for good and systemd "+
			"restarts a core that is answering perfectly well", err)
	} else if got := string(buf[:n]); got != "WATCHDOG=1" {
		t.Errorf("datagram after the lock was released = %q, want WATCHDOG=1", got)
	}
}

// The first failed TryLock still pings, and exactly one does.
//
// This is the half the wedge test above cannot see. Going back to stopping on
// the first failure keeps every other test in this file green — the difference
// is a timing margin against systemd's clock, not a behaviour a 10ms ticker
// shows — and it is the margin that matters: the interval is WatchdogSec/2, so
// a ping at t=0 and a skip at t=30 puts the next attempt at t=60+ε against a
// deadline of exactly t=60, which a Go ticker drifting late loses. One grace
// ping is what removes that boundary.
//
// pingWatchdog is driven directly rather than through Start, because that is the
// only way to hold d.mu from before the first tick. Through Start the socket can
// already hold a datagram queued before the lock landed, and "exactly one" stops
// being a thing the test can say.
func TestPingWatchdog_TheFirstFailedTryLockStillPingsAndOnlyTheFirst(t *testing.T) {
	ln := listenNotify(t, filepath.Join(t.TempDir(), "notify"))
	t.Setenv(notifySocketEnv, ln.LocalAddr().String())

	d := &Daemon{quit: make(chan struct{})}
	d.mu.Lock() // wedged before the ticker has fired once
	defer d.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		d.pingWatchdog(10 * time.Millisecond)
	}()
	t.Cleanup(func() {
		close(d.quit)
		<-done
	})

	// 300ms is thirty ping intervals, so whatever this counts is all there will
	// ever be.
	buf := make([]byte, 64)
	pings := 0
	_ = ln.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	for {
		n, _, err := ln.ReadFrom(buf)
		if err != nil {
			break
		}
		if got := string(buf[:n]); got != "WATCHDOG=1" {
			t.Fatalf("datagram = %q, want WATCHDOG=1", got)
		}
		pings++
	}

	switch {
	case pings == 0:
		t.Error("no ping at all from a daemon whose lock was merely busy — stopping on " +
			"the first failed TryLock puts the next attempt on systemd's deadline " +
			"rather than inside it, and a Go ticker drifts late")
	case pings > 1:
		t.Errorf("%d pings while the lock was held for the whole window; only the "+
			"first miss is a grace ping, and everything after it is a wedge that "+
			"has to be reported by silence", pings)
	}
}

// A unit with no WatchdogSec= sets no WATCHDOG_USEC, which is every installation
// before this release and every host that is not running systemd at all. The
// goroutine must not start: a ticker built from a zero duration panics, and a
// panic in Start is a firewall daemon that never comes up.
func TestDaemonStart_SendsNoWatchdogPingWithoutWatchdogSec(t *testing.T) {
	ln := listenNotify(t, filepath.Join(t.TempDir(), "notify"))
	t.Setenv(notifySocketEnv, ln.LocalAddr().String())
	t.Setenv(watchdogUSecEnv, "")

	cfg := newTestConfig(t)
	d := &Daemon{cfg: cfg, firewall: newTestFirewall(t, cfg), quit: make(chan struct{})}
	errCh := startDaemonGoroutine(d)
	t.Cleanup(func() {
		d.Stop()
		<-errCh
	})

	if got := readNotify(t, ln); got != "READY=1" {
		t.Fatalf("first datagram = %q, want READY=1", got)
	}

	buf := make([]byte, 64)
	_ = ln.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if n, _, err := ln.ReadFrom(buf); err == nil {
		t.Errorf("got a second datagram %q with no WATCHDOG_USEC set", buf[:n])
	}
}
