package core

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The boot restore has to run before the socket exists, and that is held by the
// order of two statements in Daemon.Start — nothing else.
//
// TestDaemonStart_RestoresAtStartup and
// TestDaemonStart_NoCommandIsServedBeforeTheRestoreHasRun both look like they
// pin it and neither does: change the restore to `go func(){ … }()` and both stay
// green, because the restore's audit write wins the race against the test's
// dial-write-read every time. That is the exact regression the commit which moved
// the restore ahead of net.Listen exists to prevent, so it gets the idiom this
// repository already uses for guarantees a runtime test cannot reach — reading
// the source and asserting on it. See
// internal/shared/docs_coverage_test.go's TestAllCommandTypesMatchesTheProtocolSource
// for the shape, including the part that matters most: a pattern which silently
// matches nothing must fail, not pass.
func TestDaemonStart_SourceRestoresBeforeItListens(t *testing.T) {
	body := daemonStartBody(t)

	restore := regexp.MustCompile(`d\.firewall\.RestoreCurrent\(`)
	listen := regexp.MustCompile(`net\.Listen\(`)

	restoreAt := restore.FindAllStringIndex(body, -1)
	listenAt := listen.FindAllStringIndex(body, -1)

	// Nothing went unparsed. Either of these matching zero times means the call
	// was renamed, moved out of Start or wrapped in something this test cannot
	// see — in which case the test has stopped guarding anything and has to say
	// so rather than pass on an empty match.
	if len(restoreAt) == 0 {
		t.Fatal("no call to d.firewall.RestoreCurrent in Daemon.Start: the boot restore has " +
			"moved or been renamed, and this guard no longer pins the ordering it exists for")
	}
	if len(listenAt) == 0 {
		t.Fatal("no call to net.Listen in Daemon.Start: the socket is created somewhere else " +
			"now, and this guard no longer pins the ordering it exists for")
	}
	// One of each, so a second call site cannot satisfy the comparison below
	// while the load-bearing one sits on the wrong side of it.
	if len(restoreAt) != 1 {
		t.Fatalf("want exactly one RestoreCurrent call in Daemon.Start, found %d; "+
			"this test compares first occurrences and cannot tell which one is the boot restore",
			len(restoreAt))
	}
	if len(listenAt) != 1 {
		t.Fatalf("want exactly one net.Listen call in Daemon.Start, found %d; "+
			"this test compares first occurrences and cannot tell which one creates the socket",
			len(listenAt))
	}

	if restoreAt[0][0] > listenAt[0][0] {
		t.Error("Daemon.Start creates the socket before it restores the stored rules. " +
			"A client can then be served while the kernel holds a half-restored firewall — " +
			"the restore has to come first, because the socket is the only thing that makes " +
			"this process observable")
	}

	// And it has to be called, not launched. A `go func(){ d.firewall.RestoreCurrent(…) }()`
	// placed above net.Listen satisfies the order and destroys the guarantee, which is
	// precisely how the two runtime tests were fooled.
	if inGoroutine(body, restoreAt[0][0]) {
		t.Error("the boot restore is inside a `go func`, so the socket can be accepting " +
			"connections while it is still running. It must be a synchronous call in " +
			"Daemon.Start; the restore's own audit write is fast enough to hide this in " +
			"every runtime test, which is why it is asserted here")
	}
}

// daemonStartBody returns the source of Daemon.Start, from the opening brace to
// the closing one in column zero.
func daemonStartBody(t *testing.T) string {
	t.Helper()
	src := coreSource(t, "daemon.go")
	const sig = "func (d *Daemon) Start() error {"
	start := strings.Index(src, sig)
	if start < 0 {
		t.Fatalf("could not find %q in daemon.go; the signature changed and this guard "+
			"is reading nothing", sig)
	}
	rest := src[start+len(sig):]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatal("could not find the end of Daemon.Start in daemon.go; this guard is " +
			"reading past the function it means to check")
	}
	body := rest[:end]
	if !strings.Contains(body, "d.cfg.SocketPath") {
		t.Fatalf("the extracted Daemon.Start body does not mention the socket path, so it "+
			"is not the function this test means to read:\n%s", body)
	}
	return body
}

// inGoroutine reports whether the byte offset sits inside a `go func` literal.
//
// Brace counting rather than go/parser: the parser would be the better tool for
// a general question, and this is a narrow one — the offset comes from a regex
// match in one known function whose only string literals and comments contain no
// braces. If that ever stops being true the counting goes wrong in the direction
// of a false alarm, which is the safe one for a guard test.
func inGoroutine(body string, off int) bool {
	depth := 0
	var goDepths []int // brace depths at which a goroutine body is currently open
	pendingGo := false
	for i := 0; i < len(body); i++ {
		if i == off {
			return len(goDepths) > 0
		}
		if strings.HasPrefix(body[i:], "go func") {
			pendingGo = true
			continue
		}
		switch body[i] {
		case '{':
			depth++
			if pendingGo {
				goDepths = append(goDepths, depth)
				pendingGo = false
			}
		case '}':
			if n := len(goDepths); n > 0 && goDepths[n-1] == depth {
				goDepths = goDepths[:n-1]
			}
			depth--
		}
	}
	return false
}

// coreSource reads a file from this package's own directory. The package's tests
// run with the package directory as the working directory, but the walk upwards
// keeps it working from a module-root invocation too — the same defence
// internal/shared's repoFile uses.
func coreSource(t *testing.T, name string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		for _, candidate := range []string{
			filepath.Join(dir, name),
			filepath.Join(dir, "internal", "core", name),
		} {
			if data, err := os.ReadFile(candidate); err == nil { // #nosec G304 -- test-only, fixed names
				return string(data)
			}
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("could not locate %s", name)
	return ""
}
