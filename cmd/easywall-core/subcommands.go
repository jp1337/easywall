package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

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

// runSubcommand executes one console subcommand and returns the process exit
// code. Writers are parameters so the tests can read what an operator would see.
//
//nolint:errcheck // every write here goes to the operator's own terminal; if that pipe is gone there is nothing further to report to
func runSubcommand(name string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("easywall-core "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "/etc/easywall/easywall.toml", "path to core config file")
	if err := fs.Parse(args); err != nil {
		return exitFailed
	}

	switch name {
	case "status", "panic", "resume":
	default:
		fmt.Fprintf(stderr, "easywall-core: unknown command %q\n\n%s", name, subcommandUsage)
		return exitFailed
	}

	cfg, err := core.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "easywall-core: cannot read %s: %v\n", *configPath, err)
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

//nolint:errcheck // every write here goes to the operator's own terminal; if that pipe is gone there is nothing further to report to
func runStatus(cfg *core.Config, stdout, stderr io.Writer) int {
	resp, err := shared.SendCommand(cfg.SocketPath, shared.Command{Type: shared.CmdGetStatus})
	if err != nil {
		fmt.Fprintf(stderr, "easywall-core: the core daemon is not answering on %s: %v\n",
			cfg.SocketPath, err)
		return exitFailed
	}
	if !resp.Success {
		fmt.Fprintf(stderr, "easywall-core: %s\n", resp.Error)
		return exitFailed
	}

	var status shared.FirewallStatus
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		fmt.Fprintf(stderr, "easywall-core: cannot read the reply: %v\n", err)
		return exitFailed
	}

	switch {
	case status.Panic:
		fmt.Fprintln(stdout, "firewall:   PANIC MODE — deliberately not enforcing")
		fmt.Fprintln(stdout, "            run `easywall-core resume` to put the stored rules back")
	case status.Active:
		fmt.Fprintln(stdout, "firewall:   enforcing")
	default:
		fmt.Fprintln(stdout, "firewall:   NOT enforcing")
	}

	fmt.Fprintf(stdout, "acceptance: %s\n", status.Acceptance)
	if status.LastApply != "" {
		fmt.Fprintf(stdout, "last apply: %s\n", status.LastApply)
	} else {
		fmt.Fprintln(stdout, "last apply: never")
	}
	if status.HasPending {
		fmt.Fprintln(stdout, "pending:    there are staged changes that are not live")
	}

	// A monitoring check has to be able to read this without parsing the words.
	// Panic mode is a decision somebody made, so it is not a failure; a machine
	// that is simply not filtering is.
	if !status.Active && !status.Panic {
		return exitNotFiltered
	}
	return exitOK
}

//nolint:errcheck // every write here goes to the operator's own terminal; if that pipe is gone there is nothing further to report to
func runPanic(cfg *core.Config, stdout, stderr io.Writer) int {
	resp, err := shared.SendCommand(cfg.SocketPath, shared.Command{Type: shared.CmdPanic})
	if err != nil {
		fmt.Fprintf(stderr, "easywall-core: the core daemon is not answering on %s: %v\n",
			cfg.SocketPath, err)
		return exitFailed
	}
	if !resp.Success {
		fmt.Fprintf(stderr, "easywall-core: %s\n", resp.Error)
		return exitFailed
	}
	fmt.Fprintln(stdout, "The firewall is down. This machine is unfiltered, and stays that way")
	fmt.Fprintln(stdout, "across a restart until you run `easywall-core resume`.")
	return exitOK
}

//nolint:errcheck // every write here goes to the operator's own terminal; if that pipe is gone there is nothing further to report to
func runResume(cfg *core.Config, stdout, stderr io.Writer) int {
	resp, err := shared.SendCommand(cfg.SocketPath, shared.Command{Type: shared.CmdResume})
	if err != nil {
		fmt.Fprintf(stderr, "easywall-core: the core daemon is not answering on %s: %v\n",
			cfg.SocketPath, err)
		return exitFailed
	}
	if !resp.Success {
		fmt.Fprintf(stderr, "easywall-core: %s\n", resp.Error)
		return exitFailed
	}
	fmt.Fprintln(stdout, "Panic mode is over and the stored rules are back in force.")
	return exitOK
}
