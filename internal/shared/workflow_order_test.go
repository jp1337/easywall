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

// The search index has to be built in both jobs, and before the upload.
//
// It is the one part of the documentation site that is not a committed artefact
// and not produced by Jekyll: it is derived from _site by a composite action, and
// nothing downstream notices its absence. Delete the `uses:` line from the deploy
// job, or move it below `Upload Pages artifact`, and every check in this
// repository still passes — while every visitor to easywall-project.org gets a
// search field whose engine answers 404, because the artefact that was uploaded
// was built before the index existed. Dropping it from the pull-request job is
// quieter still: the assertion that the index covers all 26 pages lives inside
// the action, so removing the call removes the only thing that would ever report
// a glob that stopped matching.
//
// Text and step order rather than YAML, for the reason jobBlock gives.
func TestTheSearchIndexIsBuiltBeforeThePagesUpload(t *testing.T) {
	const action = "uses: ./.github/actions/build-search-index"
	workflow := repoFile(t, ".github", "workflows", "docs.yml")

	for _, job := range []string{"build", "deploy"} {
		if !strings.Contains(jobBlock(t, workflow, job), action) {
			t.Errorf("the %s job in docs.yml does not run %s\n"+
				"  in `deploy` that publishes a site whose search engine is not there; "+
				"in `build` it removes the only check that the index covers every page",
				job, action)
		}
	}

	deploy := jobBlock(t, workflow, "deploy")
	index := strings.Index(deploy, action)
	upload := strings.Index(deploy, "uses: actions/upload-pages-artifact")
	switch {
	case upload < 0:
		t.Fatal("the deploy job no longer uploads a Pages artifact")
	case index < 0:
		// Already reported above; nothing to order.
	case index > upload:
		t.Error("the search index is built after `Upload Pages artifact`, so the " +
			"artefact that gets published is the one from before the index existed — " +
			"the site deploys green with no /pagefind/ directory on it")
	}
}

// A release candidate must not move `latest`.
//
// release.yml triggers on `v*.*.*`, and that glob matches `v2.6.0-rc1` as
// happily as `v2.6.0`. The image tag list said `latest` unconditionally, so
// publishing a candidate would have moved the tag installation/docker.md calls
// the production one, and every `docker compose pull` on `:latest` would have
// taken it. Nothing about that is visible until someone tags a candidate.
func TestLatestImageTagIsOnlyForStableReleases(t *testing.T) {
	cfg := repoFile(t, ".goreleaser.yaml")

	// Every tag list that mentions latest has to make it conditional on there
	// being no prerelease part.
	for _, line := range strings.Split(cfg, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "latest") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.Contains(trimmed, ".Prerelease") {
			t.Errorf("an image is tagged latest without asking whether this is a prerelease:\n  %s", trimmed)
		}
	}

	// And GitHub should not call a candidate the newest release either.
	if !strings.Contains(cfg, "prerelease: auto") {
		t.Error(".goreleaser.yaml does not set release.prerelease, so a candidate " +
			"is published as a full GitHub release")
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

// Every architecture that gets an image gets a package.
//
// The container has been published for linux/amd64 and linux/arm64 since it
// existed; the .deb was amd64 only, and nothing connected the two — so the gap
// was invisible in both files. It is the same shape as the release that carried
// no .deb at all: an artefact somebody assumed was there.
//
// Reading .goreleaser.yaml's platforms and release.yml's matrix and comparing
// the sets means adding an architecture to the images without adding it to the
// packages fails here, in words, rather than as a missing download six months
// later.
func TestEveryImageArchitectureAlsoGetsAPackage(t *testing.T) {
	images := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s+- linux/(\w+)\s*$`).
		FindAllStringSubmatch(repoFile(t, ".goreleaser.yaml"), -1) {
		images[m[1]] = true
	}
	if len(images) == 0 {
		t.Fatal(".goreleaser.yaml lists no image platforms; this test compares against them")
	}

	block := jobBlock(t, repoFile(t, ".github", "workflows", "release.yml"), "debian")
	packages := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s+- arch: (\w+)\s*$`).
		FindAllStringSubmatch(block, -1) {
		packages[m[1]] = true
	}
	if len(packages) == 0 {
		t.Fatal("the release's debian job has no architecture matrix")
	}

	for arch := range images {
		if !packages[arch] {
			t.Errorf("images are built for %s but no .deb is: someone on that architecture "+
				"can run the container and has no dpkg path", arch)
		}
	}
	for arch := range packages {
		if !images[arch] {
			t.Errorf("a .deb is built for %s but no image is — one of the two lists has "+
				"moved without the other", arch)
		}
	}
}

// The package job has to install what it builds, on the architecture it built
// it for, and it has to do that installation inside a container rather than
// on the runner itself.
//
// The first half is old: a cross-built package that nothing installs is how
// this repository shipped a .deb with no binaries in it for its whole
// existence.
//
// The second half is the more expensive lesson. build-deb was green through
// 2026-08-17 and red on every branch from 2026-08-18: the 2.7 boot-restore
// change made easywall-core program nftables into the kernel the moment it
// starts, and debian/postinst starts that service unconditionally. Installing
// the .deb straight onto the runner — which is what this job did until then,
// via a plain `apt-get install -y ./dist/*.deb` — handed the runner its own
// `input policy drop`. The runner does not crash; it loses its own route back
// to the Actions service, so the job hangs at whatever step happened to be
// running when the rule landed, until the platform times it out around the
// 47-minute mark. release.yml builds the identical package and was never
// touched by any of this, for one reason: it only ever builds. It never
// installs what it builds, so postinst never runs anywhere near the machine
// executing that job.
//
// So this test checks two things that both have to hold, and neither implies
// the other: the job installs the package it built (not just compiles it),
// and it does that installation somewhere other than the runner's own network
// namespace.
func TestPackageInstallsNativelyInsideAContainer(t *testing.T) {
	block := jobBlock(t, repoFile(t, ".github", "workflows", "build.yml"), "build-deb")

	for _, want := range []string{"ubuntu-24.04\n", "ubuntu-24.04-arm"} {
		if !strings.Contains(block, want) {
			t.Errorf("the package job does not run on %q, so that architecture is either "+
				"cross-built or not built at all", strings.TrimSpace(want))
		}
	}

	// The exact path (`./dist/*.deb`) is not the point — a bind mount, a `docker
	// cp`, or a renamed directory can all change it without changing the thing
	// under test. What has to hold is that some line actually runs `apt-get
	// install` against a .deb, wherever it lives.
	installLine := regexp.MustCompile(`(?m)^.*apt-get install[^\n]*\.deb.*$`).FindString(block)
	if installLine == "" {
		t.Fatal("the package job no longer installs the package it builds")
	}

	// The incident above, made permanent: that install line must run inside a
	// container, never directly against the runner.
	if !strings.Contains(installLine, "docker exec") {
		t.Errorf("the package is installed by a command that does not run through "+
			"`docker exec`:\n  %s\n"+
			"installing this package puts a firewall with input policy drop on "+
			"whatever machine runs that command — on the runner itself, that is the "+
			"runner executing the job. It loses its own route back to the Actions "+
			"service and the job hangs until the platform gives up on it.",
			strings.TrimSpace(installLine))
	}
}
