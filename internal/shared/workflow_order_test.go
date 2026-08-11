package shared

import (
	"regexp"
	"strings"
	"testing"
)

// jobBlock returns the lines of one job from a workflow file: everything from
// the job's key until the next key at the same indentation.
//
// Text rather than YAML, because the repository has no YAML dependency and this
// question — which step comes first — is answerable from the order of the lines.
func jobBlock(t *testing.T, workflow, job string) string {
	t.Helper()
	lines := strings.Split(workflow, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "  "+job+":") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("no job %q in the workflow", job)
	}
	nextJob := regexp.MustCompile(`^  [a-zA-Z0-9_-]+:`)
	for i := start + 1; i < len(lines); i++ {
		if nextJob.MatchString(lines[i]) {
			return strings.Join(lines[start:i], "\n")
		}
	}
	return strings.Join(lines[start:], "\n")
}

// CodeQL has to find the Go toolchain already installed when it starts.
//
// codeql-action/init puts a wrapper `go` on PATH so it can trace the build. A
// setup-go that runs after it prepends the real toolchain and the wrapper is
// never called, so the build the analysis was pointed at is not the build it
// sees. CodeQL reports that as a warning, not a failure, which is the worst
// shape available: the job stays green and the scan is looking elsewhere.
//
//	Go was installed after the `codeql-action/init` Action was run.
//	Expected `which go` to return
//	/home/runner/work/_temp/codeql-action-go-tracing/bin/go, but got
//	/opt/hostedtoolcache/go/1.25.12/x64/bin/go
//
// The natural order to write is init first, which is how it got this way. This
// test exists so that writing it naturally again fails here instead of in a
// warning nobody reads.
func TestCodeQLSeesTheGoToolchainItTraces(t *testing.T) {
	block := jobBlock(t, repoFile(t, ".github", "workflows", "security.yml"), "codeql")

	// The `uses:` lines, not any mention: the comment above these steps names
	// both actions, and matching bare names measured the prose instead of the
	// order. The test caught that about itself first.
	setupGo := strings.Index(block, "uses: actions/setup-go")
	initCodeQL := strings.Index(block, "uses: github/codeql-action/init")

	switch {
	case setupGo < 0:
		t.Fatal("the codeql job does not install Go at all")
	case initCodeQL < 0:
		t.Fatal("the codeql job does not initialise CodeQL")
	case setupGo > initCodeQL:
		t.Error("actions/setup-go runs after codeql-action/init, so CodeQL's " +
			"build tracing is bypassed and the analysis does not see the build")
	}
}

// gosec has to be asked for test files before a build tag means anything.
//
// Everything behind the integration tag is a _test.go file, and gosec skips
// test files unless -tests is given — so `-tags integration` on its own scans
// exactly the same 41 files as no flags at all. Measured both ways: 41 files,
// 9,808 lines, 23 issues. The flag looked like coverage and was not.
func TestGosecScansTheIntegrationCode(t *testing.T) {
	block := jobBlock(t, repoFile(t, ".github", "workflows", "security.yml"), "gosec")

	// The command, not the comment above it. The comment explains both flags,
	// so searching the block found them whether or not they were being passed —
	// the same way the CodeQL check above first measured its own prose.
	var cmd string
	for _, line := range strings.Split(block, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "run: gosec") {
			cmd = trimmed
			break
		}
	}
	if cmd == "" {
		t.Fatal("the gosec job does not run gosec")
	}

	if !strings.Contains(cmd, "-tags integration") {
		t.Errorf("gosec does not run over the integration-tagged code:\n  %s", cmd)
	}
	if !strings.Contains(cmd, "-tests") {
		t.Errorf("gosec runs without -tests, so the build tag selects nothing — "+
			"every file behind it is a test file:\n  %s", cmd)
	}
}

// Whatever builds under CodeQL has to be the whole module, not one command.
// An analysis of half the tree reports on half the tree and says nothing about
// the rest.
func TestCodeQLBuildsEverything(t *testing.T) {
	block := jobBlock(t, repoFile(t, ".github", "workflows", "security.yml"), "codeql")
	if !strings.Contains(block, "go build ./...") {
		t.Error("the codeql job does not build the whole module between init and analyze")
	}
}
