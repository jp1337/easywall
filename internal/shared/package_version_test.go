package shared

import (
	"os"
	"path/filepath"
	"regexp"
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
