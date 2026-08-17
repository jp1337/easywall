package shared

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The Go toolchain is written down in five places, and they have to agree.
//
// They did not. The Dockerfile said golang:1.26-alpine while go.mod said 1.25.0
// and ten workflow steps pinned "1.25" — so the container that ships was built
// by a Go version no test ever ran against, and nothing could see it. The cause
// was mechanical: Dependabot understands a Docker tag and none of the other
// four, so the one place it could reach walked off on its own.
//
// go.mod's `toolchain` line is the source now. actions/setup-go reads it in
// preference to the `go` directive, Renovate keeps it current, and this test is
// what makes "current" mean all five places rather than whichever one a tool
// happened to understand.
//
// Note what is deliberately *not* checked: the `go` directive. That is the
// minimum language version the module claims — it tracks the oldest Go this code
// compiles with (1.25, for http.NewCrossOriginProtection), not the toolchain we
// build with, and the two are allowed to differ.
func TestGoToolchainIsTheSameEverywhere(t *testing.T) {
	full, short := toolchainVersion(t)

	t.Run("Dockerfile builds with it", func(t *testing.T) {
		dockerfile := repoFile(t, "Dockerfile")
		m := regexp.MustCompile(`(?m)^FROM golang:([0-9.]+)-alpine`).FindStringSubmatch(dockerfile)
		if m == nil {
			t.Fatal("no `FROM golang:<version>-alpine` in the Dockerfile; the builder stage has to " +
				"name an exact patch so it can be compared with go.mod")
		}
		if m[1] != full {
			t.Errorf("the Dockerfile builds with Go %s, go.mod's toolchain is %s\n"+
				"  the published image would be compiled by a Go version nothing tests with", m[1], full)
		}
	})

	t.Run("the Debian build-dependency admits it", func(t *testing.T) {
		control := repoFile(t, "debian", "control")
		m := regexp.MustCompile(`golang-go \(>= 2:([0-9.]+)~?\)`).FindStringSubmatch(control)
		if m == nil {
			t.Fatal("debian/control has no versioned golang-go build-dependency")
		}
		if m[1] != short {
			t.Errorf("debian/control asks for golang-go %s, the toolchain is %s\n"+
				"  a build with GOTOOLCHAIN=local refuses anything older than the toolchain line", m[1], short)
		}
	})

	// The prose a person reads before installing anything. Each entry is a place
	// that states a *requirement*; sentences naming the release an API arrived in
	// are a different kind of claim and stay where they are — see the guard below.
	t.Run("the documentation says the same", func(t *testing.T) {
		for _, pin := range []struct {
			file    []string
			pattern string
		}{
			{[]string{"CONTRIBUTING.md"}, `Install Go ([0-9.]+)\+`},
			{[]string{"README.md"}, `From source — Go ([0-9.]+)\+`},
			{[]string{"README.md"}, `\| Go ([0-9.]+), single binary \|`},
			{[]string{"docs", "index.md"}, `<strong>Go ([0-9.]+)</strong>`},
			{[]string{"docs", "_docs", "installation", "manual.md"}, `Go ([0-9.]+)\+ on the \*\*build\*\* machine`},
			{[]string{"docs", "_diagrams", "install-choice.mmd"}, `From source<br/>Go ([0-9.]+) \+ make`},
		} {
			name := strings.Join(pin.file, "/")
			m := regexp.MustCompile(pin.pattern).FindStringSubmatch(repoFile(t, pin.file...))
			if m == nil {
				t.Errorf("%s no longer states the Go version in the form %q — "+
					"renovate.json edits it by that shape, so it would silently stop being updated",
					name, pin.pattern)
				continue
			}
			if m[1] != short {
				t.Errorf("%s tells the reader to install Go %s, the toolchain is %s", name, m[1], short)
			}
		}
	})

	// The one that keeps the source single. Ten literal pins are how this drifted
	// in the first place, and they are easy to reintroduce: `go-version: "1.26"`
	// is what every example on the internet shows.
	t.Run("no workflow pins a version of its own", func(t *testing.T) {
		for _, wf := range []string{"build.yml", "test.yml", "security.yml", "release.yml",
			"publish-edge.yml", "docs.yml"} {
			body := repoFile(t, ".github", "workflows", wf)
			for i, line := range strings.Split(body, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "#") {
					continue
				}
				if strings.HasPrefix(trimmed, "go-version:") {
					t.Errorf("%s:%d pins the toolchain itself:\n  %s\n"+
						"  use `go-version-file: go.mod` — one source, which is the point",
						wf, i+1, trimmed)
				}
			}
		}
	})
}

// A version arriving in Go is a fact about that release, not a requirement.
//
// net/http.CrossOriginProtection appeared in Go 1.25 and will have appeared in
// Go 1.25 for ever; five pages say so. A regex over "Go 1.25" — in renovate.json
// or in a careless edit here — would rewrite them into a lie the next time the
// toolchain moves. This test states which sentences are off limits.
func TestTheCSRFClaimNamesTheReleaseItArrivedIn(t *testing.T) {
	const arrived = "1.25"
	for _, f := range [][]string{
		{"README.md"},
		{"SECURITY.md"},
		{"docs", "_docs", "security.md"},
		{"docs", "_docs", "configuration.md"},
		{"docs", "_docs", "installation", "manual.md"},
	} {
		name := strings.Join(f, "/")
		body := repoFile(t, f...)
		if !strings.Contains(body, "CrossOriginProtection") {
			continue // the page stopped mentioning it; nothing to protect
		}
		if !strings.Contains(body, "Go "+arrived) {
			t.Errorf("%s mentions CrossOriginProtection but no longer says Go %s\n"+
				"  that API arrived in %s; the sentence is a fact about the release, "+
				"not a build requirement that follows the toolchain", name, arrived, arrived)
		}
	}
}

// toolchainVersion returns go.mod's toolchain as "1.26.5" and "1.26".
func toolchainVersion(t *testing.T) (full, short string) {
	t.Helper()
	gomod := repoFile(t, "go.mod")
	m := regexp.MustCompile(`(?m)^toolchain go([0-9]+\.[0-9]+(?:\.[0-9]+)?)`).FindStringSubmatch(gomod)
	if m == nil {
		t.Fatal("go.mod has no toolchain directive; it is the single source every " +
			"other place in this test is compared against, and setup-go reads it")
	}
	full = m[1]
	parts := strings.Split(full, ".")
	return full, strings.Join(parts[:2], ".")
}

// What renovate.json will actually rewrite, applied to the tree it will rewrite.
//
// A configuration that validates is not a configuration that matches the right
// lines. Both of these were live for one Renovate run:
//
//   - README.md carried an HTML comment saying badges "cannot drift the way a
//     hand-written Go 1.25+ could" — a sentence warning about a stale pin, with
//     a stale pin in it. The prose manager matched it, so Renovate had found a
//     dependency that would be out of date for ever.
//   - debian/control repeated its own Build-Depends line verbatim in the comment
//     above it, so the same file matched twice.
//
// Neither is visible in a config validator; both are obvious the moment the
// patterns are run over the files. This test runs them, and requires every value
// they capture to be the toolchain — anything else is a line Renovate would edit
// into a lie, or a pin that has been left behind.
func TestRenovateEditsOnlyTheGoPinsItShould(t *testing.T) {
	_, short := toolchainVersion(t)

	var cfg struct {
		CustomManagers []struct {
			Description         string   `json:"description"`
			ManagerFilePatterns []string `json:"managerFilePatterns"`
			MatchStrings        []string `json:"matchStrings"`
		} `json:"customManagers"`
	}
	if err := json.Unmarshal([]byte(repoFile(t, "renovate.json")), &cfg); err != nil {
		t.Fatalf("renovate.json does not parse: %v", err)
	}
	if len(cfg.CustomManagers) == 0 {
		t.Fatal("renovate.json declares no customManagers; the documentation pins " +
			"and debian/control are then updated by nobody")
	}

	root := repoRootDir(t)
	for _, cm := range cfg.CustomManagers {
		for _, raw := range cm.ManagerFilePatterns {
			// Renovate writes a regex between slashes; a bare string is a glob.
			pathRe := regexp.MustCompile(strings.Trim(raw, "/"))
			for _, rel := range trackedFiles(t, root) {
				if !pathRe.MatchString(rel) {
					continue
				}
				body := readRepoPath(t, root, rel)
				for _, ms := range cm.MatchStrings {
					re, err := regexp.Compile(ms)
					if err != nil {
						t.Errorf("matchStrings %q does not compile in Go's regexp: %v", ms, err)
						continue
					}
					idx := re.SubexpIndex("currentValue")
					if idx < 0 {
						t.Errorf("matchStrings %q has no currentValue group, so Renovate "+
							"cannot know what to replace", ms)
						continue
					}
					for _, m := range re.FindAllStringSubmatch(body, -1) {
						if m[idx] != short {
							t.Errorf("%s: renovate.json manages a Go pin reading %q, the toolchain is %q\n"+
								"  pattern: %s\n"+
								"  either the pin was left behind, or the pattern caught a sentence "+
								"that is not a pin — see the note on this test", rel, m[idx], short, ms)
						}
					}
				}
			}
		}
	}
}

// repoRootDir returns the directory holding go.mod.
func repoRootDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("no go.mod above the package directory")
	return ""
}

// trackedFiles lists what git tracks, so the scan follows the repository rather
// than whatever happens to be lying in the working tree.
func trackedFiles(t *testing.T, root string) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "ls-files").Output()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

// readRepoPath reads one tracked file by its repository-relative path.
func readRepoPath(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel)) // #nosec G304 -- a path git listed
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}
