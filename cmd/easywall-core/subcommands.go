package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"slices"
	"strings"
	"syscall"
	"time"

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

// subcommand is one console command and the single line of help an operator
// sees for it.
type subcommand struct{ name, help string }

// subcommands is every console command, in the order the help prints them.
//
// One list, rather than a hand-written usage string standing beside a
// membership switch. The help block is the only place an operator finds out
// that a command exists at all — `health` is unreachable knowledge if it is
// dispatched and not printed — and until 2.17 the two were maintained by hand
// with nothing checking that they agreed. TestUsageNamesEverySubcommand hangs
// off this list, so a sixth command cannot be added without becoming
// discoverable.
var subcommands = []subcommand{
	{"status", "report whether the firewall is enforcing, and since when"},
	{"health", "report whether the firewall is doing what it says"},
	{"selftest", "prove the rule builder against this kernel"},
	{"panic", "take the firewall down and record that it was deliberate"},
	{"resume", "end panic mode and put the stored rules back"},
}

// subcommandUsage is the operator-facing help. Built from the list above, and
// also what main.go prints for `-h`.
var subcommandUsage = buildSubcommandUsage()

func buildSubcommandUsage() string {
	var b strings.Builder
	b.WriteString("easywall-core <command> [-config path]\n\n")
	for _, c := range subcommands {
		// Eight, because `selftest` is the longest name there is room for on one
		// column; a longer one would push its own description out of line and be
		// visible immediately in the help rather than only in a diff.
		_, _ = fmt.Fprintf(&b, "  %-8s %s\n", c.name, c.help)
	}
	return b.String()
}

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
const auditUserNoDaemon = "console-no-daemon"

// runSubcommand executes one console subcommand and returns the process exit
// code. Writers are parameters so the tests can read what an operator would see.
func runSubcommand(name string, args []string, stdout, stderr io.Writer) int {
	// Before the flags, not after: `selftest` is the first subcommand with a
	// flag of its own, and which flags exist now depends on which command was
	// asked for. Rejecting the name first also means `easywall-core frobnicate
	// -whatever` reports the unknown command rather than the unknown flag,
	// which is the mistake that was actually made.
	if !slices.ContainsFunc(subcommands, func(c subcommand) bool { return c.name == name }) {
		_, _ = fmt.Fprintf(stderr, "easywall-core: unknown command %q\n\n%s", name, subcommandUsage)
		return exitFailed
	}

	flags := flag.NewFlagSet("easywall-core "+name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "/etc/easywall/easywall.toml", "path to core config file")

	// A plain bool rather than the pointer flags.Bool returns, and registered
	// only for the command it belongs to: every other command reads a usable
	// false, where a nil *bool would be a panic one line of refactoring away.
	var ifStale bool
	if name == "selftest" {
		flags.BoolVar(&ifStale, "if-stale", false,
			"run only when the recorded version or kernel differs from this one")
	}

	if err := flags.Parse(args); err != nil {
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
	case "health":
		return runHealth(cfg, stdout, stderr)
	case "selftest":
		return runSelftest(cfg, ifStale, stdout, stderr)
	case "panic":
		return runPanic(cfg, stdout, stderr)
	case "resume":
		return runResume(cfg, stdout, stderr)
	default:
		// Unreachable — the membership check above passed, so this name is in
		// `subcommands`. Spelled out rather than left as `default: return
		// runResume(...)`, which is how this switch ended when there were three
		// commands. With five that shorthand is no longer merely terse: a sixth
		// command added to the list and forgotten here would have silently run
		// `resume`, and resume on a machine somebody deliberately unfiltered
		// puts the rules back. A loud refusal beats a plausible wrong action.
		_, _ = fmt.Fprintf(stderr,
			"easywall-core: %q is in the command list but nothing dispatches it\n", name)
		return exitFailed
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

// describeProof is the one line both `health` and `selftest` print about what
// was last proven, and against what.
//
// Every part of it is omitted when it is empty rather than printed as a label
// with nothing after it. That is not tidiness. A GET_HEALTH reply is not only
// ever built by the privileged core: the demo builds the same typed reply with
// an `unprovable` stamp whose kernel is the zero value, because internal/web
// has no path to core.KernelRelease() — it imports internal/core nowhere, and
// that separation is the whole design. On a real host an `unprovable` stamp
// does carry a kernel, so the empty one is not a state this can dismiss as
// impossible. `kernel: ` with nothing after it would read as a bug in the one
// release whose entire subject is a firewall not making false statements about
// itself.
func describeProof(version, kernel string, result shared.SelftestResult, at time.Time) string {
	if result == "" {
		return "never recorded"
	}
	line := string(result)
	if version != "" {
		line += " for " + version
	}
	if kernel != "" {
		line += " on " + kernel
	}
	if !at.IsZero() {
		line += " at " + at.UTC().Format(time.RFC3339)
	}
	return line
}

// printStamp prints what the proof found, including its detail.
//
// SelftestStamp.Detail reaches an operator here and nowhere else. It is
// deliberately absent from HealthResult — /healthz is unauthenticated by
// necessity, and the detail names a port number and the claim that failed, so
// the reply cannot carry it. This console runs on the machine, so it can. A
// field written by RunSelftest and read by nobody is a field the next
// maintainer deletes as dead weight, having no way to tell it from some.
func printStamp(w io.Writer, stamp shared.SelftestStamp) {
	_, _ = fmt.Fprintf(w, "selftest:   %s\n",
		describeProof(stamp.Version, stamp.Kernel, stamp.Result, stamp.At))
	if stamp.Detail != "" {
		_, _ = fmt.Fprintf(w, "detail:     %s\n", stamp.Detail)
	}
}

// runHealth asks the one question every other surface renders.
//
// For five releases the input chain's `ct state established,related accept`
// matched no packet — the conntrack masks were byte-reversed — so the stateful
// half enforced nothing while `status`, the dashboard and the audit log all
// reported the firewall active. Every signal easywall had was about intent;
// none was about effect. This is the console end of the one that is.
//
// The exit codes are the contract a monitoring check and a Docker HEALTHCHECK
// depend on: ok → 0, degraded → 1, fail → 2. They deliberately diverge from
// `status` under panic mode, where status exits 0 and health exits 1. That was
// ruled on rather than overlooked: a machine somebody chose to unfilter is in a
// state somebody chose, so a console asking after *intent* is right to be
// quiet — but a monitoring system asking after *health* is asking a different
// question, and answering it closes carried-forward's entry open since 2.7,
// "a forgotten panic mode is invisible to monitoring; a panic nobody remembers
// never pages anyone". TestHealthAndStatusDisagreeUnderPanic pins the
// divergence so it stays a decision instead of being rediscovered as a bug.
func runHealth(cfg *core.Config, stdout, stderr io.Writer) int {
	resp, err := shared.SendCommand(cfg.SocketPath, shared.Command{Type: shared.CmdGetHealth})
	if err != nil {
		if !daemonAbsent(err) {
			_, _ = fmt.Fprintf(stderr, "easywall-core: the core daemon is not answering on %s: %v\n",
				cfg.SocketPath, err)
			return exitFailed
		}
		// exitNotFiltered, the same code `status` uses for the same situation
		// and for the same reason: being unable to confirm that a firewall is
		// up is not the same as it being up. Health is read out of the kernel,
		// and with no daemon there is nobody to read it.
		// The constant, not the word: this line and the exit code below are the
		// same claim, and a literal here would drift the moment the enum's
		// spelling changed while a script grepping for it kept passing.
		_, _ = fmt.Fprintf(stdout, "health:     %s\n", shared.HealthFail)
		_, _ = fmt.Fprintln(stdout, "reason:     the core daemon is not running, so nothing can")
		_, _ = fmt.Fprintln(stdout, "            confirm what the kernel is holding")
		return exitNotFiltered
	}
	if !resp.Success {
		_, _ = fmt.Fprintf(stderr, "easywall-core: %s\n", resp.Error)
		return exitFailed
	}

	var health shared.HealthResult
	if err := json.Unmarshal(resp.Data, &health); err != nil {
		_, _ = fmt.Fprintf(stderr, "easywall-core: cannot read the reply: %v\n", err)
		return exitFailed
	}

	_, _ = fmt.Fprintf(stdout, "health:     %s\n", health.State)
	_, _ = fmt.Fprintf(stdout, "reason:     %s\n", health.Reason)
	_, _ = fmt.Fprintf(stdout, "selftest:   %s\n",
		describeProof(health.Selftest.Version, health.Selftest.Kernel,
			health.Selftest.Result, health.Selftest.At))

	switch health.State {
	case shared.HealthOK:
		return exitOK
	case shared.HealthDegraded:
		return exitFailed
	case shared.HealthFail:
		return exitNotFiltered
	default:
		// A state this binary has no code for is not a healthy one. It means a
		// daemon newer than this console, which on a machine where the package
		// upgraded and the binary on PATH did not is exactly the moment an
		// operator must not read a 0.
		_, _ = fmt.Fprintf(stderr,
			"easywall-core: the daemon answered with a state this binary does not know (%q)\n",
			health.State)
		return exitFailed
	}
}

// selftestRunner is core.RunSelftest in production and a counter in the tests.
//
// Injected because the two things this subcommand decides — whether to run at
// all, and what to exit with — need no kernel, while the proof itself builds a
// network namespace and wants CAP_SYS_ADMIN. The alternative was a test that
// skips when it cannot get one, and a t.Skip whose precondition is absent reads
// exactly like a pass in CI output. internal/core's own tests prove the provers
// against a real kernel.
var selftestRunner = core.RunSelftest

// runSelftest proves the rule builder against this kernel and records what it
// found.
//
// `--if-stale` is what the systemd unit runs before the daemon starts: the
// defect class layer C prevents is a *build* defect, identical on every start
// of the same binary, so proving it once per version and kernel is exactly
// enough. The bare subcommand is the maintainer's invocation after changing a
// builder, and it always runs and always rewrites the stamp — the whole reason
// to run it by hand is to distrust what is recorded.
//
// `unprovable` exits 0. The systemd unit lists SuccessExitStatus=0 1 2 so it
// cannot fail a boot either way, but a non-zero code meaning "this kernel
// cannot be asked" would show red in `systemctl status` on every container and
// every hardened host — and it is the *ordinary* production state, since the
// daemon holds CAP_NET_ADMIN and not CAP_SYS_ADMIN. A red that means nothing
// gets ignored within a week, and takes the reds that mean something with it.
func runSelftest(cfg *core.Config, ifStale bool, stdout, stderr io.Writer) int {
	store := core.NewStampStore(cfg.SelftestStampPath())

	if ifStale && !store.Stale(shared.CurrentVersion, core.KernelRelease()) {
		stamp := store.Read()
		printStamp(stdout, stamp)
		_, _ = fmt.Fprintln(stdout,
			"            already recorded for this version and kernel; not run again")
		return proofExitCode(stamp.Result)
	}

	stamp := selftestRunner()
	if err := store.Write(stamp); err != nil {
		// The proof ran; only the record of it failed. Reporting that and going
		// on to print the result is deliberate — whoever ran this asked what the
		// builder does on this kernel, and an unwritable data directory does not
		// change the answer. A missing stamp reads as stale, so the next start
		// simply proves it again.
		_, _ = fmt.Fprintf(stderr,
			"easywall-core: the self-test ran, but its result could not be recorded: %v\n", err)
	}
	printStamp(stdout, stamp)
	return proofExitCode(stamp.Result)
}

// proofExitCode maps a recorded result onto an exit code. Only a false claim is
// a failure; see runSelftest for why `unprovable` is not.
func proofExitCode(result shared.SelftestResult) int {
	if result == shared.SelftestFailed {
		return exitFailed
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
		// listener is closed and unlinked. What makes those survivable is the
		// marker going on disk before this teardown *and* the daemon re-reading
		// it after each write to the table, not before only: a check before a
		// write cannot see a marker that appears during it, and the loser of
		// that race would be a machine filtering with panic mode recorded, which
		// every status surface reports as "deliberately not enforcing". See
		// Firewall.panicLandedDuringWrite for the second half.
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
