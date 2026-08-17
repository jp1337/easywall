package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	t.Cleanup(func() { _ = ln.Close() })

	done := make(chan struct{})
	t.Cleanup(func() { <-done })
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
	dir := t.TempDir()
	path := filepath.Join(dir, "easywall.toml")
	body := "socket_path = \"" + socketPath + "\"\n" +
		"data_dir = \"" + dir + "\"\n" +
		"log_dir = \"" + dir + "\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
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
