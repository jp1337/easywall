package core

import (
	"net"
	"os"
	"strconv"
	"time"
)

// sd_notify, in the twenty lines it actually is.
//
// systemd's readiness protocol is one datagram to the AF_UNIX socket named by
// $NOTIFY_SOCKET. There is no library here and there does not need to be: the
// whole protocol this daemon uses is two literals, READY=1 and WATCHDOG=1.
// Pulling in a dependency to send a nine-byte datagram would add a supply chain
// to the privileged process for something net.Dial already does.
//
// Why it matters at all: easywall-core.service was Type=simple, which makes the
// unit "active (running)" the instant exec() succeeds. easywall-web.service has
// After=easywall-core.service, and what it is actually waiting for is the
// control socket at /run/easywall/core.sock — which does not exist until
// Daemon.Start has run net.Listen and handed the socket to the easywall group,
// hundreds of milliseconds and one full rule restore later. Under Type=simple
// systemd was free to start the web process into a socket that was not there
// yet, and the web process reports that as "the core is unreachable" on every
// page. Type=notify plus NotifyReady() below makes "active" mean the thing the
// ordering was always assuming it meant.

// notifySocketEnv and watchdogUSecEnv are systemd's, not easywall's, so they are
// deliberately not in shared/env.go's table: that table is the operator-facing
// configuration layer and TestNoEnvVarTargetsARuleField and friends police what
// may appear in it. These two are set by the service manager for its own
// protocol and are never written by a human.
const (
	notifySocketEnv = "NOTIFY_SOCKET"
	watchdogUSecEnv = "WATCHDOG_USEC"
)

// notify sends one datagram to $NOTIFY_SOCKET and drops every error on the
// floor, deliberately.
//
// Nothing in this daemon depends on the notification arriving. An empty
// $NOTIFY_SOCKET is the ordinary case for a Docker container, a `make install`
// host running the binary under something other than systemd, and a developer
// running ./bin/easywall-core by hand — so a warning would fire on every one of
// those, and a warning on the watchdog path would fire once per interval for
// the entire life of a misconfigured installation. That is exactly the failure
// mode UsageStore.lastParseErr exists to prevent: a journal that is one
// repeated line, in which the line that matters cannot be found.
func notify(state string) {
	addr := os.Getenv(notifySocketEnv)
	if addr == "" {
		return
	}

	// No translation of an abstract address, and that is the finding rather than
	// an omission.
	//
	// systemd uses an abstract socket in several configurations — user managers,
	// some container runtimes — and sd_notify(3) spells that address with a
	// leading '@' standing for a NUL first byte in the sockaddr_un. Every C
	// implementation therefore replaces the byte, and three lines doing the same
	// thing stood here.
	//
	// They were dead. Go's syscall layer already accepts either spelling:
	// SockaddrUnix.sockaddr zeroes a leading '@' *and* shortens the length for a
	// leading NUL, so both produce the identical address. Deleting the branch
	// left TestNotifyReadyReachesAnAbstractSocket green — which is what that test
	// is now for. The branch survived one round of review as insurance against a
	// future Go release or a non-Linux port changing that; neither is a thing
	// this daemon can reach, since CLONE_NEWNET and netlink make it Linux-only by
	// construction, and speculative code is what this repository's own rules
	// reject. The knowledge is worth keeping; the branch was not.
	//
	// #nosec G704 -- gosec's taint analysis reads any dial target that came from
	// the environment as attacker-controlled. Three things make it not one here:
	// $NOTIFY_SOCKET is set by systemd in the service's own environment and no
	// request can reach it; "unixgram" is a filesystem socket, so there is no
	// host, no port and no route for a request to be smuggled out over; and this
	// process is already root, so anyone able to set that variable in it could
	// do anything the daemon can do without needing this line at all.
	conn, err := net.Dial("unixgram", addr)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	// A deadline, because the watchdog goroutine holds a d.wg slot that Stop()
	// waits on. $NOTIFY_SOCKET is a datagram socket with a finite queue, and a
	// write to a full one blocks: PID 1 not draining it would park this
	// goroutine, and shutdown would then wait on it until systemd sent SIGKILL.
	// It matches the canonical C implementation and needs systemd itself to
	// stop reading, so it is a hazard rather than a defect — and the cure is
	// two lines. Whatever happens, an unsent notification is already dropped on
	// the floor here by design.
	_ = conn.SetWriteDeadline(time.Now().Add(notifyWriteTimeout))
	_, _ = conn.Write([]byte(state))
}

// notifyWriteTimeout bounds the one write above. Generous by the standards of a
// local datagram socket that is normally never full, and short enough that
// shutdown is not visibly delayed by it.
const notifyWriteTimeout = 2 * time.Second

// NotifyReady tells systemd the daemon is up. Call it once, and only after the
// thing Type=notify is asserting is true — see Daemon.Start.
func NotifyReady() { notify("READY=1") }

// NotifyWatchdog resets systemd's WatchdogSec timer. Call it only when the
// daemon has just established that it is still answering; a ping sent
// unconditionally is a watchdog that cannot fire.
func NotifyWatchdog() { notify("WATCHDOG=1") }

// WatchdogInterval is how often to ping, or 0 when systemd has not asked for a
// watchdog at all.
//
// systemd sets WATCHDOG_USEC from the unit's WatchdogSec=. Half of it is the
// interval systemd's own documentation recommends, and the reason is that the
// deadline is measured on systemd's clock: a ping sent at exactly WatchdogSec
// races the timer, and one late scheduling of this goroutine kills a healthy
// firewall daemon. Half leaves a whole missed ping of slack.
func WatchdogInterval() time.Duration {
	usec, err := strconv.ParseInt(os.Getenv(watchdogUSecEnv), 10, 64)
	if err != nil || usec <= 0 {
		return 0
	}
	return time.Duration(usec) * time.Microsecond / 2
}
