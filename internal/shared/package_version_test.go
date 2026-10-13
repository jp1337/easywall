package shared

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The Debian package's version is the top entry of debian/changelog, and it is
// the last version in the repository that nothing tied to anything.
//
// docs/_config.yml has TestDocsVersionMatchesRelease, the Go toolchain has
// TestGoToolchainIsTheSameEverywhere, and the binaries get theirs from
// -ldflags. dpkg-parsechangelog is what debian/rules reads for both the package
// version and the version compiled into the binaries it builds, so a release
// that bumps CurrentVersion and forgets this file produces a package whose
// contents disagree with its metadata — and `apt upgrade` offers nothing,
// because the version it compares has not moved.
//
// The release workflow does compare the two, but only at the upload step:
// GoReleaser has by then published the images, the archives and the GitHub
// release, so the failure arrives after most of the release has happened and
// leaves it half done. This is the same check, at the moment it is cheap.
func TestThePackageVersionIsTheReleaseVersion(t *testing.T) {
	path := filepath.Join(repoRoot(t), "debian", "changelog")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read debian/changelog: %v", err)
	}

	// The first line of a Debian changelog: `package (version) suite; urgency=…`
	m := regexp.MustCompile(`^easywall \(([^)]+)\)`).FindSubmatch(data)
	if m == nil {
		t.Fatalf("debian/changelog does not start with an entry dpkg-parsechangelog can read:\n%.80s", data)
	}

	if got := string(m[1]); got != CurrentVersion {
		t.Errorf("debian/changelog is at %q, shared.CurrentVersion is %q\n"+
			"  debian/rules takes the package version *and* the -ldflags version from this file, "+
			"so the two would disagree about what the .deb contains, and the release would "+
			"fail at the upload step with the images already published", got, CurrentVersion)
	}
}

// Every -ldflags -X in the build files has to name shared.CurrentVersion, and
// CurrentVersion has to be a variable. The linker ignores an -X whose target
// does not exist, so a typo builds cleanly and the binary reports the literal
// in the source — which is exactly what the Debian job's version check
// compared it against until 2.25.1, so a misspelt target in debian/rules
// stayed green. The Dockerfile's and .goreleaser.yaml's are never asked at all.
// Measured: built with -X …shared.CurrentVersoin=2.25.0, easywall-core
// reported 2.25.0, and the check passed.
func TestEveryLinkerVersionFlagNamesTheVariable(t *testing.T) {
	const target = "github.com/jp1337/easywall/internal/shared.CurrentVersion"
	_ = &CurrentVersion // does not compile if it goes back to being a const

	root := repoRootDir(t)
	flag := regexp.MustCompile(`-X[ =]+([A-Za-z0-9_./-]+)=`)
	files := 0
	for _, f := range trackedFiles(t, root) {
		switch {
		case f == "Makefile", f == "Dockerfile", f == ".goreleaser.yaml", f == "debian/rules",
			strings.HasPrefix(f, ".github/workflows/"):
		default:
			continue
		}
		found := false
		for _, m := range flag.FindAllStringSubmatch(readRepoPath(t, root, f), -1) {
			found = true
			if m[1] != target {
				t.Errorf("%s: -X %s names nothing the version lives in; the linker ignores it "+
					"silently and the binary reports the source default. Want -X %s", f, m[1], target)
			}
		}
		if found {
			files++
		}
	}
	// Makefile, Dockerfile, .goreleaser.yaml, debian/rules, build.yml.
	if files < 5 {
		t.Errorf("found -X in %d build files, want at least 5: the pattern has stopped matching", files)
	}
}
