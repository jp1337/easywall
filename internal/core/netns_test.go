package core

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

// The first line of easywall's security architecture is that no subprocess runs
// in the privileged path. The veth harness that found the conntrack defect is
// test code using unshare, nsenter, ip, ping, bash and timeout — six binaries,
// unremarkable in a test and unacceptable in the root daemon.
//
// So the shipped harness execs exactly one thing: itself. This walks the AST
// rather than grepping, because a comment naming a binary must not fail and a
// string built by concatenation must not pass.
//
// A file that does not exist yet is skipped rather than fatal: selftest.go
// arrives in a later change, and a guard that goes red on a file nobody has
// written is a guard people learn to ignore. The counter is the price of that
// leniency — if both files were ever renamed the walk would inspect nothing at
// all and this test would pass by inspecting nothing, which is the one failure
// mode a skip introduces.
//
// os/exec is not the only way to start a program, and a guard whose subject is
// "no subprocess" cannot stop at the spelling that happens to be in use.
// syscall.Exec, syscall.ForkExec, os.StartProcess and an exec.Cmd built as a
// composite literal all run a program without ever naming exec.Command, and all
// four passed this test until the review pointed at the gap between it and its
// own comment.
//
// The second round of that same gap: unix.Exec and unix.ForkExec, in a package
// netns.go already imports — so no new import made the evasion visible — and
// any of the five under an import alias, because the map was keyed on the
// identifier at the call site. Both are closed by keying on the import path,
// which is the one part of a spelling a file cannot rename.
func TestSelftestUsesNoExternalBinary(t *testing.T) {
	// Every other way to start a program. None of them takes the one argument
	// shape below, so they are refused outright rather than inspected: if the
	// harness ever needs one, that is a decision to make in review and not a
	// call to slip past a guard.
	//
	// Keyed by import *path* and not by the identifier at the call site, which
	// closes two holes at once. golang.org/x/sys/unix has its own Exec and
	// ForkExec and is already imported by netns.go, so that evasion needed no
	// new import for a reviewer to notice — and `import sys "syscall"` renamed
	// its way past a map keyed on the identifier. The path is what the alias
	// cannot change.
	otherWaysToStartAProgram := map[string]string{
		"syscall.Exec":                   "replaces this process with another program",
		"syscall.ForkExec":               "starts another program",
		"golang.org/x/sys/unix.Exec":     "replaces this process with another program",
		"golang.org/x/sys/unix.ForkExec": "starts another program",
		"os.StartProcess":                "starts another program",
	}

	parsed := 0
	for _, file := range []string{"netns.go", "selftest.go"} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, nil, 0)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		parsed++
		// The identifier a package is reached by in this file, resolved to its
		// import path. `exec`, `sys` and `unix` are all just names a file
		// chose; the path is the thing being forbidden.
		pkgPath := map[string]string{}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			name := path[strings.LastIndex(path, "/")+1:]
			if imp.Name != nil {
				name = imp.Name.Name
			}
			pkgPath[name] = path
		}
		ast.Inspect(f, func(n ast.Node) bool {
			// exec.Cmd{Path: …} runs whatever Path names, and Start is then a
			// method call on a value this walk would never have looked at.
			if comp, ok := n.(*ast.CompositeLit); ok {
				if sel, ok := comp.Type.(*ast.SelectorExpr); ok {
					if pkg, ok := sel.X.(*ast.Ident); ok && pkgPath[pkg.Name] == "os/exec" && sel.Sel.Name == "Cmd" {
						t.Errorf("%s:%d: an exec.Cmd composite literal; build the command with "+
							"exec.Command(\"/proc/self/exe\") so that the program this package "+
							"execs stays visible to this guard",
							file, fset.Position(comp.Pos()).Line)
					}
				}
				return true
			}

			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if why, forbidden := otherWaysToStartAProgram[pkgPath[pkg.Name]+"."+sel.Sel.Name]; forbidden {
				t.Errorf("%s:%d: %s.%s %s; the only program this package may exec is "+
					"/proc/self/exe, through exec.Command",
					file, fset.Position(call.Pos()).Line, pkg.Name, sel.Sel.Name, why)
				return true
			}
			if pkgPath[pkg.Name] != "os/exec" {
				return true
			}
			if sel.Sel.Name != "Command" && sel.Sel.Name != "CommandContext" {
				return true
			}
			// The program argument is the first for Command, the second for
			// CommandContext.
			argIdx := 0
			if sel.Sel.Name == "CommandContext" {
				argIdx = 1
			}
			lit, ok := call.Args[argIdx].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				t.Errorf("%s:%d: exec.%s with a non-literal program; the only "+
					"program this package may exec is /proc/self/exe",
					file, fset.Position(call.Pos()).Line, sel.Sel.Name)
				return true
			}
			if strings.Trim(lit.Value, `"`) != "/proc/self/exe" {
				t.Errorf("%s:%d: exec.%s(%s); the only program this package may "+
					"exec is /proc/self/exe",
					file, fset.Position(call.Pos()).Line, sel.Sel.Name, lit.Value)
			}
			return true
		})
	}
	if parsed == 0 {
		t.Fatal("this guard inspected no file at all: netns.go and selftest.go are both " +
			"missing, so nothing pins the harness to a single exec target")
	}
}

// openFds counts this process's file descriptors, which is how a leak is
// visible without waiting for one to matter.
func openFds(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatalf("counting open descriptors: %v", err)
	}
	return len(entries)
}

// The refusal path, which is the *usual* path in production and therefore the
// one most worth a test that runs everywhere.
//
// easywall-core.service grants CAP_NET_ADMIN and bounds the set to it;
// CLONE_NEWNET needs CAP_SYS_ADMIN. So on an ordinary systemd installation, and
// in almost any container, NewHarness cannot build anything and the self-test
// has to report "unprovable" rather than "broken". This substitutes startPeer
// so that both refusals can be driven with no kernel, no capability and no
// TestMain — end-to-end is impossible here, because internal/core's TestMain
// belongs to the integration build.
//
// The descriptor count is the assertion worth the most. NewHarness owns
// /proc/<pid>/ns/net for the harness's whole life, and a refusal that leaked
// one fd per daemon start would stay invisible until a long-lived process ran
// out of them.
func TestHarnessRefusalIsACleanSentinel(t *testing.T) {
	original := startPeer
	t.Cleanup(func() { startPeer = original })

	t.Run("the clone is refused", func(t *testing.T) {
		before := openFds(t)
		startPeer = func() (*exec.Cmd, *os.File, *os.File, error) {
			// What clone(CLONE_NEWNET) returns without CAP_SYS_ADMIN. The real
			// startPeer has already closed its pipes by the time it returns an
			// error, so the fake opens none either.
			return nil, nil, nil, syscall.EPERM
		}

		h, err := NewHarness()
		if !errors.Is(err, ErrNamespaceUnavailable) {
			t.Errorf("err = %v; a refused clone must satisfy errors.Is(err, "+
				"ErrNamespaceUnavailable), because that is what tells the self-test "+
				"to say \"unprovable\" instead of \"the firewall is wrong\"", err)
		}
		if h != nil {
			t.Error("NewHarness returned a non-nil *Harness alongside an error; " +
				"a half-built harness must not be reachable")
			h.Close()
		}
		if after := openFds(t); after != before {
			t.Errorf("open descriptors went from %d to %d across a refused start", before, after)
		}
	})

	t.Run("the peer never says ready", func(t *testing.T) {
		before := openFds(t)

		// A live-looking launch: two real pipes, and a command that was never
		// started, so Close finds Process == nil and reaps nothing.
		peerOut, weWrite, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe: %v", err)
		}
		weRead, peerIn, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe: %v", err)
		}
		defer func() { _, _ = weWrite.Close(), weRead.Close() }()

		// Written before the call, so the ready read finds a line waiting and
		// the test does not spend harnessReadyTimeout getting there.
		if _, err := weWrite.WriteString("i am not ready\n"); err != nil {
			t.Fatalf("priming the peer's pipe: %v", err)
		}
		startPeer = func() (*exec.Cmd, *os.File, *os.File, error) {
			return exec.Command("/proc/self/exe"), peerIn, peerOut, nil
		}

		h, err := NewHarness()
		if !errors.Is(err, ErrNamespaceUnavailable) {
			t.Errorf("err = %v; a peer that never reports ready is the same refusal "+
				"as a refused clone and must carry the same sentinel", err)
		}
		if h != nil {
			t.Error("NewHarness returned a non-nil *Harness alongside an error")
			h.Close()
		}
		// peerIn and peerOut were handed to the harness; the two ends this test
		// kept are closed by the defer above, after this count.
		if after := openFds(t); after != before+2 {
			t.Errorf("open descriptors went from %d to %d; NewHarness was handed two "+
				"descriptors and must close both when it gives up", before, after)
		}
	})
}
