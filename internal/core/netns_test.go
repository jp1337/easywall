package core

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
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
func TestSelftestUsesNoExternalBinary(t *testing.T) {
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
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "exec" {
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
