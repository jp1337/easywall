package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/core"
	"github.com/jp1337/easywall/internal/shared"
)

// coreSocket serves one command from a fake daemon and returns the socket path.
func coreSocket(t *testing.T, resp shared.Response) string {
	t.Helper()
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "core.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})

	// Registration order is load-bearing, and t.Cleanup runs LIFO: the wait has
	// to be registered *before* the close so that the close runs first. The
	// other way round — which is how this helper was written — the wait blocks
	// on an Accept that nothing will ever return from, and a test that reaches
	// this socket without dialling it hangs instead of failing. That was found
	// by mutation: deleting `case "health"` from the dispatch switch left every
	// health test with a socket nobody dials, and `go test` sat there until the
	// package timeout. A hang in CI reads like nothing at all, the same way an
	// absent-precondition t.Skip reads like a pass.
	t.Cleanup(func() { <-done })
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.ReadAll(io.LimitReader(conn, 1<<20))
		out, _ := json.Marshal(resp)
		_, _ = conn.Write(out)
	}()
	return socketPath
}

// writeConfig writes a minimal core config pointing at socketPath.
func writeConfig(t *testing.T, socketPath string) string {
	t.Helper()
	path, _ := writeConfigDir(t, socketPath)
	return path
}

// writeConfigDir is writeConfig, also returning the data directory — which is
// where the self-test stamp lives, so `selftest`'s tests need both.
func writeConfigDir(t *testing.T, socketPath string) (cfgPath, dir string) {
	t.Helper()
	dir = t.TempDir()
	cfgPath = filepath.Join(dir, "easywall.toml")
	body := "socket_path = \"" + socketPath + "\"\n" +
		"data_dir = \"" + dir + "\"\n" +
		"log_dir = \"" + dir + "\"\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return cfgPath, dir
}

func TestRunSubcommand_StatusPrintsWhatTheKernelHolds(t *testing.T) {
	status, err := json.Marshal(shared.FirewallStatus{
		Active:     true,
		Acceptance: shared.AcceptanceIdle,
		LastApply:  "2026-08-16T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, coreSocket(t, shared.Response{Success: true, Data: status}))

	var out, errOut bytes.Buffer
	if code := runSubcommand("status", []string{"-config", cfgPath}, &out, &errOut); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, errOut.String())
	}
	got := out.String()
	for _, want := range []string{"enforcing", "2026-08-16T10:00:00Z"} {
		if !strings.Contains(got, want) {
			t.Errorf("status output does not mention %q:\n%s", want, got)
		}
	}
}

// The exit code is the part a script reads. Not enforcing must not be 0: a
// monitoring check that treats an unfiltered machine as healthy is worse than
// no check.
func TestRunSubcommand_StatusExitsNonZeroWhenNotEnforcing(t *testing.T) {
	status, err := json.Marshal(shared.FirewallStatus{Active: false, Acceptance: shared.AcceptanceIdle})
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, coreSocket(t, shared.Response{Success: true, Data: status}))

	var out, errOut bytes.Buffer
	if code := runSubcommand("status", []string{"-config", cfgPath}, &out, &errOut); code == 0 {
		t.Errorf("exit code 0 for a machine that is not filtering:\n%s", out.String())
	}
}

func TestRunSubcommand_PanicReportsSuccess(t *testing.T) {
	cfgPath := writeConfig(t, coreSocket(t, shared.Response{Success: true}))

	var out, errOut bytes.Buffer
	if code := runSubcommand("panic", []string{"-config", cfgPath}, &out, &errOut); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "unfiltered") {
		t.Errorf("panic must say plainly what state the machine is in now:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "easywall-core resume") {
		t.Errorf("panic must name the way back:\n%s", out.String())
	}
}

func TestRunSubcommand_ResumeReportsSuccess(t *testing.T) {
	cfgPath := writeConfig(t, coreSocket(t, shared.Response{Success: true}))

	var out, errOut bytes.Buffer
	if code := runSubcommand("resume", []string{"-config", cfgPath}, &out, &errOut); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "back in force") {
		t.Errorf("resume must say plainly that the stored rules are back in force:\n%s", out.String())
	}
}

// An error from the core is the operator's, not a stack trace.
func TestRunSubcommand_CoreErrorIsReported(t *testing.T) {
	cfgPath := writeConfig(t, coreSocket(t, shared.Response{Success: false, Error: "tear down the table: no permission"}))

	var out, errOut bytes.Buffer
	if code := runSubcommand("panic", []string{"-config", cfgPath}, &out, &errOut); code == 0 {
		t.Error("a refused command must not exit 0")
	}
	if !strings.Contains(errOut.String(), "no permission") {
		t.Errorf("the core's reason must reach stderr:\n%s", errOut.String())
	}
}

func TestRunSubcommand_UnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runSubcommand("frobnicate", nil, &out, &errOut); code == 0 {
		t.Error("an unknown subcommand must not exit 0")
	}
	if !strings.Contains(errOut.String(), "status") {
		t.Errorf("the error should list the subcommands that do exist:\n%s", errOut.String())
	}
}

// The help block is the only place an operator finds out a command exists.
// This used to be the line above — a substring check for "status" — which
// proved that one command was named and nothing about the rest; 2.17 added two,
// and neither would have been noticed missing.
//
// Line-anchored on purpose. A plain substring check for "panic" is satisfied by
// resume's description ("end panic mode and put the stored rules back"), so
// deleting the panic line entirely would still have passed it. That is the
// shape of defect this release has produced four times: a test that goes green
// for the wrong reason.
func TestUsageNamesEverySubcommand(t *testing.T) {
	// A guard that inspects nothing passes. An empty or truncated list would
	// make the loop below vacuous, so the list's own size is asserted first.
	if len(subcommands) < 5 {
		t.Fatalf("the command list has %d entries; status, health, selftest, panic and "+
			"resume all exist, so a shorter list means this guard is inspecting nothing",
			len(subcommands))
	}

	var out, errOut bytes.Buffer
	runSubcommand("frobnicate", nil, &out, &errOut)
	unknown := errOut.String()

	for _, c := range subcommands {
		line := "\n  " + c.name + " "
		if !strings.Contains(subcommandUsage, line) {
			t.Errorf("the usage text has no line for %q — a command that is dispatched and "+
				"not printed is a command nobody runs:\n%s", c.name, subcommandUsage)
		}
		if c.help == "" {
			t.Errorf("%q has no description in the help", c.name)
		}
		if !strings.Contains(unknown, line) {
			t.Errorf("the unknown-command error does not list %q:\n%s", c.name, unknown)
		}
	}
}

// The table is the only place a command is written down, so the compiler now
// enforces most of what used to need a test: an entry carries its own run
// function, and "listed but not dispatched" is no longer reachable by editing
// one copy of the five names and not another. One hole is left that the
// compiler cannot see — a nil `run` — and this is the walk that closes it, at
// test time rather than at an operator's shell.
//
// Not a substitute for the runtime refusal in runSubcommand: this test is what
// makes the mistake impossible to commit, and that refusal is what makes it
// harmless if it ever reaches a binary anyway. The previous shape of "nothing
// dispatches this" was `default: return runResume(...)`, which put the rules
// back on a machine somebody had deliberately unfiltered.
func TestEverySubcommandIsRunnable(t *testing.T) {
	// A guard that inspects nothing passes: an empty table would make every
	// loop in this file vacuous.
	if len(subcommands) < 5 {
		t.Fatalf("the command table has %d entries; status, health, selftest, panic and "+
			"resume all exist, so a shorter table means this guard is inspecting nothing",
			len(subcommands))
	}
	for i, c := range subcommands {
		if c.name == "" {
			t.Errorf("entry %d has no name", i)
		}
		if c.help == "" {
			t.Errorf("%q has no description, so the help block cannot describe it", c.name)
		}
		if c.run == nil {
			t.Errorf("%q has no function to run — the table is the dispatch, so an entry "+
				"without one is a command that exists in the help and does nothing", c.name)
		}
	}
}

// And if one ever reaches a shipped binary, it is refused rather than ignored.
// Loud beats plausible: this is the position the old `default: return
// runResume(...)` occupied.
func TestASubcommandWithNoFunctionIsRefused(t *testing.T) {
	previous := subcommands
	subcommands = append(slices.Clone(subcommands), subcommand{"frobnicate", "does nothing at all", nil})
	t.Cleanup(func() { subcommands = previous })

	var out, errOut bytes.Buffer
	if code := runSubcommand("frobnicate", nil, &out, &errOut); code == exitOK {
		t.Errorf("an entry with no function exited 0:\n%s%s", out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "no function to run") {
		t.Errorf("it must say what is wrong, not merely fail:\n%s", errOut.String())
	}
	// And it must refuse before touching anything: no config was read, and the
	// default config path does not exist on a test machine, so a message about
	// /etc/easywall would mean the refusal came too late.
	if strings.Contains(errOut.String(), "/etc/easywall") {
		t.Errorf("it read the config before refusing:\n%s", errOut.String())
	}
}

// health's exit codes are the contract a monitoring check and a Docker
// HEALTHCHECK both read. They are not status's: status exits 0 under panic
// deliberately, because a machine somebody chose to unfilter is in a state
// somebody chose, and health answers a different question.
func TestHealthExitCodes(t *testing.T) {
	cases := []struct {
		state  shared.HealthState
		reason shared.HealthReason
		want   int
	}{
		{shared.HealthOK, shared.HealthReasonHealthy, exitOK},
		{shared.HealthDegraded, shared.HealthReasonStatefulDead, exitFailed},
		{shared.HealthFail, shared.HealthReasonNotEnforcing, exitNotFiltered},
	}

	for _, c := range cases {
		t.Run(string(c.state), func(t *testing.T) {
			health, err := json.Marshal(shared.HealthResult{
				State:  c.state,
				Reason: c.reason,
				Selftest: shared.HealthSelftest{
					Version: "2.17.0",
					Kernel:  "6.11.0-19-generic",
					Result:  shared.SelftestPassed,
					At:      time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC),
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			cfgPath := writeConfig(t, coreSocket(t, shared.Response{Success: true, Data: health}))

			var out, errOut bytes.Buffer
			if code := runSubcommand("health", []string{"-config", cfgPath}, &out, &errOut); code != c.want {
				t.Errorf("health for state %q = %d, want %d (stderr %s)",
					c.state, code, c.want, errOut.String())
			}
			// The state and the reason both have to be readable without
			// parsing the exit code back into words: an operator reads the text
			// and a script reads the code, and the two must agree.
			for _, want := range []string{string(c.state), string(c.reason), "2.17.0", "6.11.0-19-generic"} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("health output does not mention %q:\n%s", want, out.String())
				}
			}
		})
	}
}

// A state this binary has no code for is not a healthy one. That happens on a
// machine where the package upgraded and the binary on PATH did not, which is
// exactly the moment an operator must not read a 0.
func TestHealthRefusesAStateItDoesNotKnow(t *testing.T) {
	health, err := json.Marshal(shared.HealthResult{State: "splendid", Reason: shared.HealthReasonHealthy})
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, coreSocket(t, shared.Response{Success: true, Data: health}))

	var out, errOut bytes.Buffer
	if code := runSubcommand("health", []string{"-config", cfgPath}, &out, &errOut); code == exitOK {
		t.Errorf("an unknown state exited 0:\n%s%s", out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "splendid") {
		t.Errorf("it must name the state it could not read:\n%s", errOut.String())
	}
}

// Panic mode: status says 0, health says 1. The divergence closes
// carried-forward's 2.7 entry — "a forgotten panic mode is invisible to
// monitoring" — and is pinned here so it stays a decision rather than being
// rediscovered as a bug and "fixed" in either direction.
func TestHealthAndStatusDisagreeUnderPanic(t *testing.T) {
	status, err := json.Marshal(shared.FirewallStatus{Panic: true, Acceptance: shared.AcceptanceIdle})
	if err != nil {
		t.Fatal(err)
	}
	statusCfg := writeConfig(t, coreSocket(t, shared.Response{Success: true, Data: status}))

	var statusOut, statusErr bytes.Buffer
	if code := runSubcommand("status", []string{"-config", statusCfg}, &statusOut, &statusErr); code != exitOK {
		t.Errorf("status under panic = %d, want %d — a machine somebody chose to unfilter is "+
			"in a state somebody chose, and that exit code was ruled on (stderr %s)",
			code, exitOK, statusErr.String())
	}

	health, err := json.Marshal(shared.HealthResult{
		State:  shared.HealthDegraded,
		Reason: shared.HealthReasonPanic,
	})
	if err != nil {
		t.Fatal(err)
	}
	healthCfg := writeConfig(t, coreSocket(t, shared.Response{Success: true, Data: health}))

	var healthOut, healthErr bytes.Buffer
	if code := runSubcommand("health", []string{"-config", healthCfg}, &healthOut, &healthErr); code != exitFailed {
		t.Errorf("health under panic = %d, want %d — a monitoring system asking after health "+
			"asks a different question from a console asking after intent (stderr %s)",
			code, exitFailed, healthErr.String())
	}
	if !strings.Contains(healthOut.String(), string(shared.HealthReasonPanic)) {
		t.Errorf("health must name panic as the reason, or the 1 is unexplained:\n%s",
			healthOut.String())
	}
}

// With no daemon, health exits 2 — the code status uses for the same situation
// and for the same reason: being unable to confirm a firewall is up is not the
// same as it being up.
func TestHealthWithoutADaemon(t *testing.T) {
	cfgPath := writeConfig(t, filepath.Join(t.TempDir(), "absent.sock"))

	var out, errOut bytes.Buffer
	if code := runSubcommand("health", []string{"-config", cfgPath}, &out, &errOut); code != exitNotFiltered {
		t.Errorf("health with no daemon = %d, want %d (stderr %s)",
			code, exitNotFiltered, errOut.String())
	}
	if !strings.Contains(out.String(), "not running") {
		t.Errorf("it must say the daemon is not running:\n%s", out.String())
	}
	// The text and the exit code are the same claim, so the text has to carry
	// the state as well: a script greps the word and a monitoring check reads
	// the code, and the two must not be able to disagree.
	if !strings.Contains(out.String(), string(shared.HealthFail)) {
		t.Errorf("it must name the state, not only the cause:\n%s", out.String())
	}
}

// A stamp whose kernel is empty must render no kernel at all, not an empty
// label. The demo core answers GET_HEALTH with exactly that: internal/web has
// no path to the privileged core.KernelRelease(), by design.
func TestHealthOmitsAnEmptyKernel(t *testing.T) {
	health, err := json.Marshal(shared.HealthResult{
		State:  shared.HealthOK,
		Reason: shared.HealthReasonHealthy,
		Selftest: shared.HealthSelftest{
			Version: "2.17.0",
			Result:  shared.SelftestUnprovable,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, coreSocket(t, shared.Response{Success: true, Data: health}))

	var out, errOut bytes.Buffer
	if code := runSubcommand("health", []string{"-config", cfgPath}, &out, &errOut); code != exitOK {
		t.Fatalf("exit code %d, stderr: %s", code, errOut.String())
	}
	if strings.Contains(out.String(), " on ") {
		t.Errorf("an empty kernel must render nothing, not a label with nothing after it:\n%s",
			out.String())
	}
	// And it still says what it does know, or "omit the empty one" would be
	// satisfied by printing nothing at all.
	for _, want := range []string{string(shared.SelftestUnprovable), "2.17.0"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("health output does not mention %q:\n%s", want, out.String())
		}
	}
}

// fakeSelftest replaces the prover with a counter for the duration of one test.
// The subcommand's job is the stamp comparison and the exit code; the proof
// itself needs a network namespace and CAP_SYS_ADMIN, and internal/core's
// integration tests are where it is exercised against a real kernel.
func fakeSelftest(t *testing.T, stamp shared.SelftestStamp) *int {
	t.Helper()
	calls := 0
	previous := selftestRunner
	selftestRunner = func() shared.SelftestStamp {
		calls++
		return stamp
	}
	t.Cleanup(func() { selftestRunner = previous })
	return &calls
}

// writeStamp records a proof for this exact version and kernel, which is what
// StampStore.Stale compares against.
func writeStamp(t *testing.T, dir string, stamp shared.SelftestStamp) {
	t.Helper()
	data, err := json.Marshal(stamp)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "selftest.json"), data, 0o600); err != nil {
		t.Fatalf("write stamp: %v", err)
	}
}

func freshStamp() shared.SelftestStamp {
	return shared.SelftestStamp{
		Version: shared.CurrentVersion,
		Kernel:  core.KernelRelease(),
		Result:  shared.SelftestPassed,
		At:      time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
	}
}

// --if-stale is what the systemd unit runs on every start. The defect class it
// proves against is a build defect — identical on every start of the same
// binary — so proving it once per version and kernel is exactly enough, and
// running it again on every boot would spend twelve seconds of every start on a
// question already answered.
func TestSelftestIfStaleSkipsAFreshStamp(t *testing.T) {
	cfgPath, dir := writeConfigDir(t, filepath.Join(t.TempDir(), "absent.sock"))
	writeStamp(t, dir, freshStamp())
	calls := fakeSelftest(t, shared.SelftestStamp{Result: shared.SelftestFailed})

	var out, errOut bytes.Buffer
	code := runSubcommand("selftest", []string{"-config", cfgPath, "-if-stale"}, &out, &errOut)

	if *calls != 0 {
		t.Errorf("the proof ran %d times against a stamp that already matches this version "+
			"and kernel; --if-stale exists so a start does not pay for it twice", *calls)
	}
	if code != exitOK {
		t.Errorf("exit code %d, want %d — the recorded proof passed (stderr %s)",
			code, exitOK, errOut.String())
	}
	if !strings.Contains(out.String(), string(shared.SelftestPassed)) {
		t.Errorf("it must still report what is recorded:\n%s", out.String())
	}
}

// And the bare subcommand always runs, because that is the maintainer's
// invocation after changing a builder: the whole reason to run it by hand is to
// distrust what is recorded.
func TestSelftestWithoutIfStaleAlwaysRuns(t *testing.T) {
	cfgPath, dir := writeConfigDir(t, filepath.Join(t.TempDir(), "absent.sock"))
	writeStamp(t, dir, freshStamp())
	calls := fakeSelftest(t, shared.SelftestStamp{
		Version: shared.CurrentVersion,
		Kernel:  core.KernelRelease(),
		Result:  shared.SelftestUnprovable,
		At:      time.Date(2026, 9, 8, 9, 0, 0, 0, time.UTC),
		Detail:  "a reply on an established connection passes — clone: operation not permitted",
	})

	var out, errOut bytes.Buffer
	code := runSubcommand("selftest", []string{"-config", cfgPath}, &out, &errOut)

	if *calls != 1 {
		t.Fatalf("the proof ran %d times, want 1 — the bare subcommand must not consult the "+
			"stamp (stderr %s)", *calls, errOut.String())
	}
	if code != exitOK {
		t.Errorf("exit code %d, want %d", code, exitOK)
	}

	// And it rewrote the stamp: a run whose result is not recorded leaves the
	// daemon and every health surface reading the old verdict.
	data, err := os.ReadFile(filepath.Join(dir, "selftest.json"))
	if err != nil {
		t.Fatalf("read stamp: %v", err)
	}
	var stored shared.SelftestStamp
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("stamp is not JSON: %s", data)
	}
	if stored.Result != shared.SelftestUnprovable {
		t.Errorf("the stored stamp still reads %q; the bare subcommand must always rewrite it",
			stored.Result)
	}
}

// SelftestStamp.Detail is deliberately absent from HealthResult, because
// /healthz is unauthenticated by necessity and the detail names a port number
// and the claim that failed. This console is where it reaches a human, and it
// is the only place — a field written by RunSelftest and read by nobody is a
// field the next maintainer deletes as dead weight, having no way to tell it
// from some.
func TestSelftestPrintsTheDetail(t *testing.T) {
	cfgPath, _ := writeConfigDir(t, filepath.Join(t.TempDir(), "absent.sock"))
	const detail = "an open port accepts a connection — port 12227 was dropped"
	fakeSelftest(t, shared.SelftestStamp{
		Version: shared.CurrentVersion,
		Kernel:  core.KernelRelease(),
		Result:  shared.SelftestFailed,
		At:      time.Now().UTC(),
		Detail:  detail,
	})

	var out, errOut bytes.Buffer
	code := runSubcommand("selftest", []string{"-config", cfgPath}, &out, &errOut)

	if !strings.Contains(out.String(), detail) {
		t.Errorf("the detail is the only thing that says *which* claim failed, and this is the "+
			"one surface that carries it:\n%s", out.String())
	}
	if code != exitFailed {
		t.Errorf("a failed proof = %d, want %d (stderr %s)", code, exitFailed, errOut.String())
	}
}

// unprovable exits 0. The systemd unit lists SuccessExitStatus=0 1 2 so it
// cannot fail a boot either way, but a non-zero code meaning "this kernel
// cannot be asked" would show red in `systemctl status` on every container and
// every hardened host — and it is the ordinary production state, since the
// daemon holds CAP_NET_ADMIN and not CAP_SYS_ADMIN. A red that means nothing
// gets ignored within a week, and takes the reds that mean something with it.
func TestSelftestUnprovableExitsZero(t *testing.T) {
	cfgPath, _ := writeConfigDir(t, filepath.Join(t.TempDir(), "absent.sock"))
	fakeSelftest(t, shared.SelftestStamp{
		Version: shared.CurrentVersion,
		Kernel:  core.KernelRelease(),
		Result:  shared.SelftestUnprovable,
		At:      time.Now().UTC(),
		Detail:  "a reply on an established connection passes — clone: operation not permitted",
	})

	var out, errOut bytes.Buffer
	if code := runSubcommand("selftest", []string{"-config", cfgPath}, &out, &errOut); code != exitOK {
		t.Errorf("unprovable = %d, want %d — being unable to prove something is not the same "+
			"as it being broken (stderr %s)", code, exitOK, errOut.String())
	}
	if !strings.Contains(out.String(), string(shared.SelftestUnprovable)) {
		t.Errorf("it must say plainly that nothing was proven:\n%s", out.String())
	}
}

// A stamp recorded as failed for this version and kernel keeps exiting 1 under
// --if-stale. It is not re-run — the answer would be the same, since the class
// is a build defect — but it must not report success by virtue of being skipped.
func TestSelftestIfStaleKeepsAFailedVerdict(t *testing.T) {
	cfgPath, dir := writeConfigDir(t, filepath.Join(t.TempDir(), "absent.sock"))
	stamp := freshStamp()
	stamp.Result = shared.SelftestFailed
	stamp.Detail = "a closed port does not — port 12233 answered"
	writeStamp(t, dir, stamp)
	calls := fakeSelftest(t, shared.SelftestStamp{Result: shared.SelftestPassed})

	var out, errOut bytes.Buffer
	code := runSubcommand("selftest", []string{"-config", cfgPath, "-if-stale"}, &out, &errOut)

	if *calls != 0 {
		t.Errorf("the proof ran %d times; a recorded verdict for this version and kernel is "+
			"not stale, whichever way it went", *calls)
	}
	if code != exitFailed {
		t.Errorf("a recorded failure = %d, want %d — skipping the run must not turn a red "+
			"verdict green (stderr %s)", code, exitFailed, errOut.String())
	}
	if !strings.Contains(out.String(), stamp.Detail) {
		t.Errorf("the recorded detail must reach the operator too:\n%s", out.String())
	}
}

// With no daemon at all, panic still has to work — that is the situation it
// exists for. It writes the marker directly, so the next start of the daemon
// does not put the rules back.
func TestRunSubcommand_PanicWithoutADaemonWritesTheMarker(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "easywall.toml")
	body := "socket_path = \"" + filepath.Join(dir, "absent.sock") + "\"\n" +
		"data_dir = \"" + dir + "\"\n" +
		"log_dir = \"" + dir + "\"\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var out, errOut bytes.Buffer
	code := runSubcommand("panic", []string{"-config", cfgPath}, &out, &errOut)

	// Tearing down the table needs CAP_NET_ADMIN, which the test process does
	// not have, so the exit code depends on the environment. The marker does
	// not: it must be there either way.
	if _, err := os.Stat(filepath.Join(dir, "panic")); err != nil {
		t.Errorf("the marker must be written even with no daemon running: %v (exit %d, stderr %s)",
			err, code, errOut.String())
	}
	if !strings.Contains(out.String()+errOut.String(), "not running") {
		t.Errorf("the output must say the daemon was not running:\n%s%s", out.String(), errOut.String())
	}
}

func TestRunSubcommand_ResumeWithoutADaemonClearsTheMarker(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "easywall.toml")
	body := "socket_path = \"" + filepath.Join(dir, "absent.sock") + "\"\n" +
		"data_dir = \"" + dir + "\"\n" +
		"log_dir = \"" + dir + "\"\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "panic"), nil, 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	var out, errOut bytes.Buffer
	runSubcommand("resume", []string{"-config", cfgPath}, &out, &errOut)

	if _, err := os.Stat(filepath.Join(dir, "panic")); !os.IsNotExist(err) {
		t.Errorf("the marker must be cleared with no daemon running, stat err = %v", err)
	}
}

// status without a daemon must still say something useful, and must not exit 0.
func TestRunSubcommand_StatusWithoutADaemon(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "easywall.toml")
	body := "socket_path = \"" + filepath.Join(dir, "absent.sock") + "\"\n" +
		"data_dir = \"" + dir + "\"\n" +
		"log_dir = \"" + dir + "\"\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var out, errOut bytes.Buffer
	if code := runSubcommand("status", []string{"-config", cfgPath}, &out, &errOut); code == 0 {
		t.Error("no daemon must not exit 0")
	}
	if !strings.Contains(out.String()+errOut.String(), "not running") {
		t.Errorf("it must say the daemon is not running:\n%s%s", out.String(), errOut.String())
	}
}

// status exits 2 even when panic mode is engaged but no daemon is running,
// because the machine is not in the state it should be: nothing will restore
// the rules when the daemon starts.
func TestRunSubcommand_StatusWithPanicButNoDaemon(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "easywall.toml")
	panicMarkerPath := filepath.Join(dir, "panic")
	body := "socket_path = \"" + filepath.Join(dir, "absent.sock") + "\"\n" +
		"data_dir = \"" + dir + "\"\n" +
		"log_dir = \"" + dir + "\"\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	// Write the panic marker
	if err := os.WriteFile(panicMarkerPath, nil, 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	var out, errOut bytes.Buffer
	code := runSubcommand("status", []string{"-config", cfgPath}, &out, &errOut)
	if code != 2 {
		t.Errorf("status with panic marker but no daemon must exit 2, got %d; output:\n%s%s", code, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "panic mode") {
		t.Errorf("output must mention panic mode:\n%s", combined)
	}
}

// TestDaemonAbsent pins the classification daemonAbsent exists to make: the
// three tests above all dial a path that never existed, so on their own they
// only ever exercise the ENOENT branch. Hard-code daemonAbsent to `return
// true` and every test in this package still goes green — nothing else here
// ever produces a transport error at all, let alone one of the other shapes.
// This is the test that actually pins the verdicts, including that EACCES and
// a full accept backlog must not read as "no daemon". It does not pin the
// order of the two checks inside daemonAbsent — see the EAGAIN case below for
// why that ordering exists and why none of these cases can prove it.
func TestDaemonAbsent(t *testing.T) {
	// wrapDial mimics exactly what shared.SendCommand produces on a failed
	// dial: net.DialTimeout returns a *net.OpError wrapping a *os.SyscallError
	// wrapping the raw errno, and SendCommand wraps that again with
	// fmt.Errorf("connect to core: %w", ...).
	wrapDial := func(errno syscall.Errno) error {
		return fmt.Errorf("connect to core: %w", &net.OpError{
			Op:  "dial",
			Net: "unix",
			Err: &os.SyscallError{Syscall: "connect", Err: errno},
		})
	}
	wrapRead := func(err error) error {
		return fmt.Errorf("read response: %w", &net.OpError{Op: "read", Net: "unix", Err: err})
	}

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "a deadline exceeded reads as a timeout: a slow daemon is still a daemon",
			err:  wrapRead(os.ErrDeadlineExceeded),
			want: false,
		},
		{
			// A full accept backlog returns EAGAIN, and syscall.Errno.Timeout()
			// reports EAGAIN (with EWOULDBLOCK and ETIMEDOUT) as a timeout, so this
			// falls out on the timeout branch. It does not, however, prove the
			// timeout check has to run first: EAGAIN was never in the errno list
			// below to begin with, so swapping the two checks would not change
			// this verdict either. The ordering is defence against a *future*
			// errno joining that list which also satisfies Timeout() — none of
			// today's three (ENOENT, ECONNREFUSED, fs.ErrNotExist) do.
			name: "a full accept backlog (EAGAIN) falls out on the timeout branch, not the errno list",
			err:  wrapDial(syscall.EAGAIN),
			want: false,
		},
		{
			name: "EACCES: a live daemon whose socket denies this caller",
			err:  wrapDial(syscall.EACCES),
			want: false,
		},
		{
			name: "EPIPE: the write failed after connecting, so a daemon was there",
			err:  fmt.Errorf("send command: %w", syscall.EPIPE),
			want: false,
		},
		{
			name: "ECONNRESET: the read failed after connecting, so a daemon was there",
			err:  fmt.Errorf("read response: %w", syscall.ECONNRESET),
			want: false,
		},
		{
			name: "ENOENT wrapped the way SendCommand wraps a dial failure",
			err:  wrapDial(syscall.ENOENT),
			want: true,
		},
		{
			name: "ECONNREFUSED wrapped the way SendCommand wraps a dial failure",
			err:  wrapDial(syscall.ECONNREFUSED),
			want: true,
		},
		{
			name: "a plain error that is not an errno at all",
			err:  errors.New("parse response: unexpected end of JSON input"),
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := daemonAbsent(c.err); got != c.want {
				t.Errorf("daemonAbsent(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

// The audit user string has no map and no view function: it renders literally in
// the log table, so a rename here leaves two spellings for the same event and
// nothing notices. Pinned the honest way — the fallback path is made to write an
// entry and the entry's user is read back — rather than by comparing the constant
// with itself.
//
// resume, not panic, because resume's fallback needs no CAP_NET_ADMIN: it clears
// the marker and writes the entry, and never touches nftables.
func TestRunSubcommand_NoDaemonFallbackNamesItselfInTheAuditLog(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "easywall.toml")
	body := "socket_path = \"" + filepath.Join(dir, "absent.sock") + "\"\n" +
		"data_dir = \"" + dir + "\"\n" +
		"log_dir = \"" + dir + "\"\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "panic"), nil, 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	var out, errOut bytes.Buffer
	if code := runSubcommand("resume", []string{"-config", cfgPath}, &out, &errOut); code != exitOK {
		t.Fatalf("resume with no daemon = %d, want %d (stderr %s)", code, exitOK, errOut.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, "audit.log"))
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	var entry shared.AuditLogEntry
	line := strings.TrimSpace(string(data))
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("audit line is not JSON: %q", line)
	}
	if entry.Action != "panic_resumed" {
		t.Errorf("action = %q, want panic_resumed", entry.Action)
	}
	if entry.User != "console-no-daemon" {
		t.Errorf("audit user = %q, want console-no-daemon — this string is rendered "+
			"literally in the log table, and `console` is what the daemon-mediated "+
			"route writes for the same action", entry.User)
	}
}
