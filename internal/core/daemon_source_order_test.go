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
	// precisely how the two runtime tests were fooled. `go d.firewall.RestoreCurrent(…)`
	// with no func literal at all does the same thing and is caught by the second
	// clause; a `go` statement whose call is a method on something else — `go
	// d.restoreAtBoot()` — hides the call from this function entirely and is caught
	// by the zero-match check above instead.
	const asynchronous = "so the socket can be accepting connections while the restore is " +
		"still running. It must be a synchronous call in Daemon.Start; the restore's own " +
		"audit write is fast enough to hide this in every runtime test, which is why it is " +
		"asserted here"
	switch {
	case inGoroutine(body, restoreAt[0][0]):
		t.Error("the boot restore is inside a `go func` literal, " + asynchronous)
	case launchedByABareGo(body, restoreAt[0][0]):
		t.Error("the boot restore is launched by a bare `go` statement, " + asynchronous)
	}
}

// daemonStartBody returns the source of Daemon.Start.
func daemonStartBody(t *testing.T) string {
	t.Helper()
	body := funcBody(t, coreSource(t, "daemon.go"), "daemon.go", "func (d *Daemon) Start() error {")
	if !strings.Contains(body, "d.cfg.SocketPath") {
		t.Fatalf("the extracted Daemon.Start body does not mention the socket path, so it "+
			"is not the function this test means to read:\n%s", body)
	}
	return body
}

// funcBody returns one function's source, from the brace that opens it to the
// closing brace in column zero. Every way of failing to find it is fatal: a guard
// that reads an empty string passes everything.
func funcBody(t *testing.T, src, file, sigPrefix string) string {
	t.Helper()
	if n := strings.Count(src, sigPrefix); n != 1 {
		t.Fatalf("found %d occurrences of %q in %s, want exactly 1; the signature changed "+
			"and this guard is reading the wrong function or none at all", n, sigPrefix, file)
	}
	start := strings.Index(src, sigPrefix)
	open := strings.Index(src[start:], "{\n")
	if open < 0 {
		t.Fatalf("no body found after %q in %s", sigPrefix, file)
	}
	rest := src[start+open+2:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatalf("could not find the end of %q in %s; this guard is reading past the "+
			"function it means to check", sigPrefix, file)
	}
	return rest[:end]
}

// Every write of the rule set into the kernel is followed by the panic check.
//
// F2 was a *missing call*: `panic` could land between a marker check and the
// netlink write that followed it, and nothing looked again afterwards. The fix is
// three calls to Firewall.panicLandedDuringWrite in three functions, and nothing
// at any build tag noticed if one of them went away — delete the one in apply and
// the entire suite stays green. A defect whose shape is "the call is not there"
// needs a guard that reads the call sites, the same way the ordering in
// Daemon.Start does.
//
// The counts are minimums with a reason attached, not a tally to keep in step:
// apply and RestoreCurrent each need two, because nft.Apply can return an error
// with the ruleset already committed, so the failure branch is as much a write as
// the success path.
func TestEveryKernelWriteIsFollowedByThePanicCheck(t *testing.T) {
	const write = "f.nft.Apply("
	const check = "f.panicLandedDuringWrite("

	sources := map[string]string{
		"firewall.go": coreSource(t, "firewall.go"),
		"restore.go":  coreSource(t, "restore.go"),
	}

	// The helper itself has to exist, or every assertion below is about a name
	// nothing implements.
	if !strings.Contains(sources["restore.go"], "func (f *Firewall) panicLandedDuringWrite(") {
		t.Fatal("Firewall.panicLandedDuringWrite is not defined in restore.go; this guard " +
			"is checking calls to a function that no longer exists")
	}

	sites := []struct {
		file, sig  string
		wantChecks int
		why        string
	}{
		{"firewall.go", "func (f *Firewall) apply(", 2,
			"the success path and the failure path — nft.Apply reports errors from the " +
				"custom-rules subprocess and the final-log flush, both of which run after " +
				"the ruleset is committed"},
		{"firewall.go", "func (f *Firewall) rollback(", 1,
			"the rollback's own write races a console teardown in the window where Stop " +
				"has unlinked the socket but the acceptance rollback is still flushing"},
		{"restore.go", "func (f *Firewall) RestoreCurrent(", 2,
			"the success path and the failure path, for the same post-commit reason as apply"},
	}

	foundWrites := map[string]int{}
	foundChecks := map[string]int{}

	for _, s := range sites {
		body := funcBody(t, sources[s.file], s.file, s.sig)

		writes := indexesOf(body, write)
		if len(writes) != 1 {
			t.Errorf("%s: %s contains %d calls to %s, want exactly 1; this guard compares "+
				"against a single write and cannot tell which one the checks belong to",
				s.file, s.sig, len(writes), write)
			continue
		}
		checks := indexesOf(body, check)
		if len(checks) < s.wantChecks {
			t.Errorf("%s: %s calls %s %d time(s), want at least %d — %s",
				s.file, s.sig, check, len(checks), s.wantChecks, s.why)
		}
		for _, at := range checks {
			if at < writes[0] {
				t.Errorf("%s: %s checks the panic marker before its write to the kernel; "+
					"the point of this call is to look again *after* the write, because a "+
					"check that precedes one cannot see a marker that appears during it",
					s.file, s.sig)
			}
		}
		foundWrites[s.file] += len(writes)
		foundChecks[s.file] += len(checks)
	}

	// Nothing went unparsed. If either token appears anywhere these three
	// functions do not cover, this test is no longer looking at every write —
	// which is the failure mode that let F2 exist, so it fails rather than
	// quietly narrowing.
	for file, src := range sources {
		if got, accounted := len(indexesOf(src, write)), foundWrites[file]; got != accounted {
			t.Errorf("%s contains %d calls to %s but this guard accounted for %d; a kernel "+
				"write outside apply, rollback and RestoreCurrent has to be added to the "+
				"table in this test together with its own panic check", file, got, write, accounted)
		}
		if got, accounted := len(indexesOf(src, check)), foundChecks[file]; got != accounted {
			t.Errorf("%s contains %d calls to %s but this guard accounted for %d; a check "+
				"outside the enumerated functions is not being asserted about at all",
				file, got, check, accounted)
		}
	}
}

// indexesOf returns the offset of every occurrence of sub in s.
func indexesOf(s, sub string) []int {
	var out []int
	for i := 0; ; {
		j := strings.Index(s[i:], sub)
		if j < 0 {
			return out
		}
		out = append(out, i+j)
		i += j + len(sub)
	}
}

// goFunc matches a goroutine launched with a function literal. `\s+` rather than a
// single space because `go  func()` and `go\tfunc()` are the same statement to the
// compiler, and a guard that a stray space defeats is not a guard.
var goFunc = regexp.MustCompile(`\bgo\s+func\b`)

// bareGo matches a `go` statement that launches a call directly, with no function
// literal: `go d.firewall.RestoreCurrent(…)`. No trailing `\S`: the statement
// prefix examined below ends exactly where the call begins, so there is nothing
// after the whitespace to match — an earlier version required one and therefore
// missed the mutation it exists for.
var bareGo = regexp.MustCompile(`^\s*go\s+`)

// launchedByABareGo reports whether the statement containing the offset begins
// with `go `. This closes the evasion inGoroutine cannot see: a `go` statement
// needs no func literal, so `go d.firewall.RestoreCurrent(RestoreReasonBoot)`
// placed above net.Listen would satisfy both the ordering and the brace scan while
// making the restore asynchronous.
func launchedByABareGo(body string, off int) bool {
	start := strings.LastIndexAny(body[:off], "\n;{}")
	return bareGo.MatchString(body[start+1 : off])
}

// inGoroutine reports whether the byte offset sits inside a `go func` literal.
//
// Brace counting rather than go/parser: the parser would be the better tool for
// a general question, and this is a narrow one — the offset comes from a regex
// match in one known function whose only string literals and comments contain no
// braces. If that ever stops being true the counting goes wrong in the direction
// of a false alarm, which is the safe one for a guard test.
func inGoroutine(body string, off int) bool {
	launches := map[int]bool{}
	for _, m := range goFunc.FindAllStringIndex(body, -1) {
		launches[m[0]] = true
	}
	depth := 0
	var goDepths []int // brace depths at which a goroutine body is currently open
	pendingGo := false
	for i := 0; i < len(body); i++ {
		if i == off {
			return len(goDepths) > 0
		}
		if launches[i] {
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
