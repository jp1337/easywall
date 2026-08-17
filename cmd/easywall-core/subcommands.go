package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"syscall"

	"github.com/jp1337/easywall/internal/core"
	"github.com/jp1337/easywall/internal/shared"
)

// The console side of easywall.
//
// 2.7 restores the stored rules at startup, which takes away the escape route
// operators actually used: before it, rebooting emptied nftables and let you
// back into a machine your own rules had shut you out of. A replacement that
// only exists in the web interface would be no replacement at all — the web
// interface is the thing you cannot reach.
//
// Everything here goes over the same socket the web process uses, so there is
// never more than one writer to table inet easywall.

const subcommandUsage = `easywall-core <command> [-config path]

  status   report whether the firewall is enforcing, and since when
  panic    take the firewall down and record that it was deliberate
  resume   end panic mode and put the stored rules back
`

// exit codes. status uses them to be usable from a monitoring check.
const (
	exitOK          = 0
	exitFailed      = 1
	exitNotFiltered = 2
)

// auditUserNoDaemon marks an audit entry written by this fallback rather than
// by the daemon on the console's behalf. The daemon already writes "console"
// for a CmdPanic/CmdResume it dispatched, so panic_engaged and panic_resumed
// can come from two different processes; the log has to say which one wrote
// a given line without an operator having to guess from the timestamp.
const auditUserNoDaemon = "console (no daemon)"

// runSubcommand executes one console subcommand and returns the process exit
// code. Writers are parameters so the tests can read what an operator would see.
func runSubcommand(name string, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("easywall-core "+name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "/etc/easywall/easywall.toml", "path to core config file")
	if err := flags.Parse(args); err != nil {
		return exitFailed
	}

	switch name {
	case "status", "panic", "resume":
	default:
		_, _ = fmt.Fprintf(stderr, "easywall-core: unknown command %q\n\n%s", name, subcommandUsage)
		return exitFailed
	}

	cfg, err := core.LoadConfig(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "easywall-core: cannot read %s: %v\n", *configPath, err)
		return exitFailed
	}

	switch name {
	case "status":
		return runStatus(cfg, stdout, stderr)
	case "panic":
		return runPanic(cfg, stdout, stderr)
	default:
		return runResume(cfg, stdout, stderr)
	}
}

// daemonAbsent reports whether err from shared.SendCommand means there is no
// daemon, as opposed to a daemon that answered badly.
//
// This is the whole safety argument for touching nftables from here. The daemon
// is the only writer of table inet easywall; the CLI may write it *only* when
// there is no daemon, and a refused connection to a Unix socket is what that
// looks like. A timeout is not: a daemon that is slow is still a daemon, and two
// writers would be worse than a slow one.
func daemonAbsent(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return false
	}
	return errors.Is(err, syscall.ENOENT) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, fs.ErrNotExist)
}

// tearDownDirectly removes the easywall table without going through the daemon.
func tearDownDirectly() error {
	nft, err := core.NewNftablesManager()
	if err != nil {
		return fmt.Errorf("reach nftables: %w", err)
	}
	return nft.Reset()
}

// printPanicEngaged is what an operator sees once panic mode is in force,
// whichever route got it there — through the daemon, or with none running.
// One copy, not two, so the daemon path and the fallback path cannot drift
// apart the way only a diff would notice.
func printPanicEngaged(stdout io.Writer) {
	_, _ = fmt.Fprintln(stdout, "The firewall is down. This machine is unfiltered, and stays that way")
	_, _ = fmt.Fprintln(stdout, "across a restart until you run `easywall-core resume`.")
}

func runStatus(cfg *core.Config, stdout, stderr io.Writer) int {
	resp, err := shared.SendCommand(cfg.SocketPath, shared.Command{Type: shared.CmdGetStatus})
	if err != nil {
		if !daemonAbsent(err) {
			_, _ = fmt.Fprintf(stderr, "easywall-core: the core daemon is not answering on %s: %v\n",
				cfg.SocketPath, err)
			return exitFailed
		}
		_, _ = fmt.Fprintln(stdout, "daemon:     not running")
		if core.PanicEngaged(cfg.PanicMarkerPath()) {
			_, _ = fmt.Fprintln(stdout, "panic mode: engaged — the rules will NOT come back on start")
			_, _ = fmt.Fprintln(stdout, "            run `easywall-core resume` first")
		} else {
			_, _ = fmt.Fprintln(stdout, "panic mode: not engaged — the rules come back when the daemon starts")
		}
		return exitNotFiltered
	}
	if !resp.Success {
		_, _ = fmt.Fprintf(stderr, "easywall-core: %s\n", resp.Error)
		return exitFailed
	}

	var status shared.FirewallStatus
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		_, _ = fmt.Fprintf(stderr, "easywall-core: cannot read the reply: %v\n", err)
		return exitFailed
	}

	switch {
	case status.Panic:
		_, _ = fmt.Fprintln(stdout, "firewall:   PANIC MODE — deliberately not enforcing")
		_, _ = fmt.Fprintln(stdout, "            run `easywall-core resume` to put the stored rules back")
	case status.Active:
		_, _ = fmt.Fprintln(stdout, "firewall:   enforcing")
	default:
		_, _ = fmt.Fprintln(stdout, "firewall:   NOT enforcing")
	}

	_, _ = fmt.Fprintf(stdout, "acceptance: %s\n", status.Acceptance)
	if status.LastApply != "" {
		_, _ = fmt.Fprintf(stdout, "last apply: %s\n", status.LastApply)
	} else {
		_, _ = fmt.Fprintln(stdout, "last apply: never")
	}
	if status.HasPending {
		_, _ = fmt.Fprintln(stdout, "pending:    there are staged changes that are not live")
	}

	// A monitoring check has to be able to read this without parsing the words.
	// Panic mode is a decision somebody made, so it is not a failure; a machine
	// that is simply not filtering is.
	if !status.Active && !status.Panic {
		return exitNotFiltered
	}
	return exitOK
}

func runPanic(cfg *core.Config, stdout, stderr io.Writer) int {
	resp, err := shared.SendCommand(cfg.SocketPath, shared.Command{Type: shared.CmdPanic})
	if err != nil {
		if !daemonAbsent(err) {
			_, _ = fmt.Fprintf(stderr, "easywall-core: the core daemon is not answering on %s: %v\n",
				cfg.SocketPath, err)
			return exitFailed
		}

		// A refused socket means no daemon is accepting — not that nothing is
		// still writing. Two windows say otherwise: the boot restore runs before
		// net.Listen, and Stop's rollback can still be flushing after the
		// listener is closed and unlinked. Both are survivable because the
		// marker goes on disk before this teardown, and every daemon-side writer
		// of the table checks it first.
		//
		// The marker first, for the same reason the daemon writes it first: an
		// operator who runs this, believes it worked and reboots must not meet
		// the rules that made them run it.
		_, _ = fmt.Fprintln(stdout, "The core daemon is not running.")
		if err := core.EngagePanic(cfg.PanicMarkerPath()); err != nil {
			_, _ = fmt.Fprintf(stderr, "easywall-core: %v\n", err)
			return exitFailed
		}
		core.WriteAuditLog(cfg.AuditLogPath(), "panic_engaged", "all",
			"the firewall was taken down from the console with no daemon running", auditUserNoDaemon)
		if err := tearDownDirectly(); err != nil {
			_, _ = fmt.Fprintf(stderr, "easywall-core: panic mode is recorded, but the table "+
				"could not be torn down: %v\n", err)
			return exitFailed
		}
		printPanicEngaged(stdout)
		return exitOK
	}
	if !resp.Success {
		_, _ = fmt.Fprintf(stderr, "easywall-core: %s\n", resp.Error)
		return exitFailed
	}
	printPanicEngaged(stdout)
	return exitOK
}

func runResume(cfg *core.Config, stdout, stderr io.Writer) int {
	resp, err := shared.SendCommand(cfg.SocketPath, shared.Command{Type: shared.CmdResume})
	if err != nil {
		if !daemonAbsent(err) {
			_, _ = fmt.Fprintf(stderr, "easywall-core: the core daemon is not answering on %s: %v\n",
				cfg.SocketPath, err)
			return exitFailed
		}
		// Only the marker. Putting the rules back is the daemon's job and it will
		// do it the moment it starts — restoring from here would install a rule
		// set nothing is then supervising.
		_, _ = fmt.Fprintln(stdout, "The core daemon is not running.")
		if err := core.ClearPanic(cfg.PanicMarkerPath()); err != nil {
			_, _ = fmt.Fprintf(stderr, "easywall-core: %v\n", err)
			return exitFailed
		}
		core.WriteAuditLog(cfg.AuditLogPath(), "panic_resumed", "all",
			"panic mode was ended from the console with no daemon running", auditUserNoDaemon)
		_, _ = fmt.Fprintln(stdout, "Panic mode is over. The rules come back when easywall-core starts:")
		_, _ = fmt.Fprintln(stdout, "  systemctl start easywall-core")
		return exitOK
	}
	if !resp.Success {
		_, _ = fmt.Fprintf(stderr, "easywall-core: %s\n", resp.Error)
		return exitFailed
	}
	_, _ = fmt.Fprintln(stdout, "Panic mode is over and the stored rules are back in force.")
	return exitOK
}
