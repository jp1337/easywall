package core

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// listenNotify stands in for systemd: an unixgram socket, and a read with a
// deadline so a datagram that never arrives is a named failure rather than a
// hung package. A test that hangs reads as nothing at all in CI, which this
// release has already been bitten by once.
func listenNotify(t *testing.T, name string) *net.UnixConn {
	t.Helper()
	ln, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: name, Net: "unixgram"})
	if err != nil {
		t.Fatalf("listen on %q: %v", name, err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

func readNotify(t *testing.T, ln *net.UnixConn) string {
	t.Helper()
	buf := make([]byte, 64)
	_ = ln.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := ln.ReadFrom(buf)
	if err != nil {
		t.Fatalf("nothing arrived on the notify socket: %v", err)
	}
	return string(buf[:n])
}

// Type=simple made the core "active (running)" the instant exec succeeded —
// before the socket existed that easywall-web.service's After= is waiting for.
// Type=notify fixes it, and this is the datagram that does the fixing.
func TestNotifyReadySendsReadyOneToTheNotifySocket(t *testing.T) {
	ln := listenNotify(t, filepath.Join(t.TempDir(), "notify"))
	t.Setenv(notifySocketEnv, ln.LocalAddr().String())

	NotifyReady()

	if got := readNotify(t, ln); got != "READY=1" {
		t.Errorf("datagram = %q, want READY=1", got)
	}
}

// The same over an abstract socket, which is what systemd uses in a user
// manager and in several container runtimes. This is the case where a wrong
// address is completely silent: the datagram goes to a name nobody holds, the
// daemon reports nothing, and systemd sits at "activating" until the start
// timeout expires. Nothing else in the suite would notice.
func TestNotifyReadyReachesAnAbstractSocket(t *testing.T) {
	// Abstract names live in a kernel-wide namespace with no filesystem to keep
	// two concurrent runs apart, so the pid does the keeping.
	name := fmt.Sprintf("@easywall-notify-test-%d", os.Getpid())
	ln := listenNotify(t, name)
	t.Setenv(notifySocketEnv, name)

	NotifyReady()

	if got := readNotify(t, ln); got != "READY=1" {
		t.Errorf("datagram = %q, want READY=1", got)
	}
}

func TestNotifyWatchdogSendsWatchdogOne(t *testing.T) {
	ln := listenNotify(t, filepath.Join(t.TempDir(), "notify"))
	t.Setenv(notifySocketEnv, ln.LocalAddr().String())

	NotifyWatchdog()

	if got := readNotify(t, ln); got != "WATCHDOG=1" {
		t.Errorf("datagram = %q, want WATCHDOG=1", got)
	}
}

// No NOTIFY_SOCKET means nobody is listening — a Docker container, a developer
// running the binary by hand. It must be a no-op and must not log.
func TestNotifyIsSilentWithNoSocket(t *testing.T) {
	t.Setenv(notifySocketEnv, "")
	NotifyReady()
	NotifyWatchdog()
}

// A $NOTIFY_SOCKET pointing at nothing is the misconfigured-installation case,
// and the one where a log line would repeat once per watchdog interval for the
// life of the process. It must not panic and must not block.
func TestNotifySurvivesASocketNobodyHolds(t *testing.T) {
	t.Setenv(notifySocketEnv, filepath.Join(t.TempDir(), "nobody-is-here"))
	NotifyReady()
	NotifyWatchdog()
}

// WATCHDOG_USEC is what systemd sets from WatchdogSec. The ping interval is
// half of it, which is what systemd's own documentation recommends.
func TestWatchdogIntervalIsHalfOfWhatSystemdAsksFor(t *testing.T) {
	t.Setenv(watchdogUSecEnv, "30000000")
	if got, want := WatchdogInterval(), 15*time.Second; got != want {
		t.Errorf("WatchdogInterval() = %s, want %s", got, want)
	}
	t.Setenv(watchdogUSecEnv, "")
	if got := WatchdogInterval(); got != 0 {
		t.Errorf("with no WATCHDOG_USEC, WatchdogInterval() = %s, want 0", got)
	}
	// Garbage, and zero, are both "no watchdog" rather than "ping constantly".
	// A ticker built from a zero or negative duration panics, which would turn a
	// malformed environment into a crash-looping firewall daemon.
	for _, bad := range []string{"nonsense", "0", "-1"} {
		t.Setenv(watchdogUSecEnv, bad)
		if got := WatchdogInterval(); got != 0 {
			t.Errorf("WATCHDOG_USEC=%q gave %s, want 0", bad, got)
		}
	}
}
