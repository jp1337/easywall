package shared

import (
	"path"
	"strings"
	"testing"
)

// Every package with tests has to be named by each place that runs the unit
// tests: the Makefile's `test` target, test.yml's `test` job and debian/rules'
// dh_auto_test. All three said `./internal/...` until 2.25.1, and the 32 tests
// in cmd/easywall-core — panic, resume, status, health, selftest, the console
// an operator reaches for when the interface is gone — ran in none of them.
// Driven by `git ls-files`, so a package added later is covered without anybody
// remembering it here.
func TestEveryTestedPackageIsInTheUnitTestRuns(t *testing.T) {
	root := repoRootDir(t)
	dirs := map[string]bool{}
	for _, f := range trackedFiles(t, root) {
		if !strings.HasSuffix(f, "_test.go") || strings.HasPrefix(f, "node_modules/") {
			continue
		}
		// A file only the integration build sees belongs to test.yml's
		// integration job, which names its own packages.
		if strings.Contains(firstLine(readRepoPath(t, root, f)), "go:build integration") {
			continue
		}
		dirs[path.Dir(f)] = true
	}
	if len(dirs) == 0 {
		t.Fatal("git lists no _test.go file: this guard is reading nothing")
	}

	for _, run := range []struct{ where, block string }{
		{"the Makefile's test target", recipe(readRepoPath(t, root, "Makefile"), "test")},
		{"test.yml's test job", jobBlock(t, readRepoPath(t, root, ".github/workflows/test.yml"), "test")},
		{"debian/rules' override_dh_auto_test", recipe(readRepoPath(t, root, "debian/rules"), "override_dh_auto_test")},
	} {
		var pats []string
		for _, line := range strings.Split(run.block, "\n") {
			if !strings.Contains(line, "go test") || strings.Contains(line, "-tags integration") {
				continue
			}
			for _, f := range strings.Fields(line) {
				if strings.HasPrefix(f, "./") {
					pats = append(pats, strings.TrimPrefix(f, "./"))
				}
			}
		}
		if len(pats) == 0 {
			t.Errorf("%s runs no `go test` over a package pattern", run.where)
			continue
		}
		for dir := range dirs {
			if !coveredBy(dir, pats) {
				t.Errorf("%s does not test %s (it runs %v): its tests run nowhere there", run.where, dir, pats)
			}
		}
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// recipe returns a make target's line and the tab-indented recipe under it.
func recipe(makefile, target string) string {
	var b strings.Builder
	in := false
	for _, line := range strings.Split(makefile, "\n") {
		switch {
		case strings.HasPrefix(line, target+":"):
			in = true
		case in && !strings.HasPrefix(line, "\t"):
			return b.String()
		}
		if in {
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

// coveredBy reports whether a `go test` package pattern names dir: "x/..."
// matches x and everything below it, "..." everything, anything else itself.
func coveredBy(dir string, pats []string) bool {
	for _, p := range pats {
		if base, ok := strings.CutSuffix(p, "..."); ok {
			base = strings.TrimSuffix(base, "/")
			if base == "" || dir == base || strings.HasPrefix(dir, base+"/") {
				return true
			}
		} else if dir == strings.TrimSuffix(p, "/") {
			return true
		}
	}
	return false
}
