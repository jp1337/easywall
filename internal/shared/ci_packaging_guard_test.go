package shared

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docs-tech/packaging.md says build.yml asserts every row of its layout table.
// It did not: /var/log/easywall and /run/easywall had rows and no check, so a
// postinst that stopped making them, or made them world-readable, passed CI
// with the document still saying it was checked.
//
// Each row's paths are read from its first cell — `{a,b}` expanded, a row
// written as `…-web` skipped because it names no path on its own — and each has
// to appear in build.yml as `check <path> <owner> <mode>`.
func TestThePackageLayoutTableIsWhatBuildAsserts(t *testing.T) {
	doc := repoFile(t, "docs-tech", "packaging.md")
	build := repoFile(t, ".github", "workflows", "build.yml")

	row := regexp.MustCompile("(?m)^\\| (`/[^|]+) \\| `([a-z]+:[a-z]+)` \\| 0([0-7]{3}) \\|")
	path := regexp.MustCompile("`(/[^`]+)`")
	brace := regexp.MustCompile(`\{([^}]+)\}`)

	rows := row.FindAllStringSubmatch(doc, -1)
	if len(rows) < 8 {
		t.Fatalf("read %d layout rows from packaging.md; the table's shape changed and this test is reading nothing", len(rows))
	}
	checked := 0
	for _, r := range rows {
		for _, p := range path.FindAllStringSubmatch(r[1], -1) {
			paths := []string{p[1]}
			if m := brace.FindStringSubmatchIndex(p[1]); m != nil {
				paths = nil
				for _, alt := range strings.Split(p[1][m[2]:m[3]], ",") {
					paths = append(paths, p[1][:m[0]]+alt+p[1][m[1]:])
				}
			}
			for _, p := range paths {
				want := regexp.MustCompile(`(?m)^\s*check ` + regexp.QuoteMeta(p) + `\s+` +
					regexp.QuoteMeta(r[2]) + `\s+` + r[3] + `\s*$`)
				if !want.MatchString(build) {
					t.Errorf("packaging.md lists %s as %s 0%s, and build.yml's installed-layout step does not check it", p, r[2], r[3])
				}
				checked++
			}
		}
	}
	t.Logf("%d layout paths checked against build.yml", checked)
}

// Debian policy 10.7.3 and 10.8: a file a maintainer script creates, and a log,
// is removed on purge. postinst creates the directories that hold web.toml's
// secrets, the TLS key, the rules and the audit log; until postrm removed them
// `apt purge` left every one behind. A directory added to postinst later has to
// reach postrm too.
func TestPurgeRemovesEveryDirectoryPostinstCreates(t *testing.T) {
	postinst := repoFile(t, "debian", "postinst")
	postrm := repoFile(t, "debian", "postrm")

	rm := regexp.MustCompile(`(?m)^\s*rm -rf ((?:/[^\s]+\s*)+)$`).FindStringSubmatch(postrm)
	if rm == nil || !strings.Contains(postrm, `"$1" = purge`) {
		t.Fatal("debian/postrm has no `rm -rf` under a purge branch")
	}
	removed := strings.Fields(rm[1])

	dirs := regexp.MustCompile(`(?m)^\s*install -d .* (/[^\s]+)\s*$`).FindAllStringSubmatch(postinst, -1)
	if len(dirs) < 4 {
		t.Fatalf("read %d `install -d` lines from postinst; this test is reading nothing", len(dirs))
	}
	for _, d := range dirs {
		covered := false
		for _, r := range removed {
			if d[1] == r || strings.HasPrefix(d[1], r+"/") {
				covered = true
			}
		}
		if !covered {
			t.Errorf("postinst creates %s and postrm's purge does not remove it (removes %v)", d[1], removed)
		}
	}
}

// Every HTTPS request easywall makes — the update check, telemetry, feeds,
// country lists, ACME, notifications — verifies against the system CA bundle.
// openssl only Recommends ca-certificates, so an --no-install-recommends install
// had no bundle and every one of them failed verification.
func TestThePackageDependsOnTheCABundleItsClientsVerifyAgainst(t *testing.T) {
	client := regexp.MustCompile(`http\.(Client\{|Get\(|DefaultClient|NewRequest)`)
	found := ""
	_ = filepath.WalkDir(filepath.Join(repoRoot(t), "internal"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") || found != "" {
			return err
		}
		b, err := os.ReadFile(p) // #nosec G304 -- a path the walk found under internal/
		if err == nil && client.Match(b) {
			found = p
		}
		return err
	})
	if found == "" {
		t.Skip("no outbound HTTP client under internal/; nothing needs the CA bundle")
	}
	depends := regexp.MustCompile(`(?m)^Depends:(.*)$`).FindStringSubmatch(repoFile(t, "debian", "control"))
	if depends == nil || !regexp.MustCompile(`(^|[ ,])ca-certificates([ ,]|$)`).MatchString(depends[1]) {
		t.Errorf("%s makes outbound requests, and debian/control's Depends does not name ca-certificates", found)
	}
}

// GoReleaser publishes the release, the tarballs and :latest; the Debian legs
// come after it. The tag used to be compared with debian/changelog only in those
// legs, so a mismatch published a release without a .deb and the documented
// /releases/latest/download/easywall_amd64.deb answered 404.
func TestTheTagIsComparedBeforeAnythingIsPublished(t *testing.T) {
	workflow := repoFile(t, ".github", "workflows", "release.yml")
	needs := regexp.MustCompile(`(?m)^    needs: (.*)$`).FindStringSubmatch(jobBlock(t, workflow, "goreleaser"))
	if needs == nil || !strings.Contains(needs[1], "tag-matches-the-tree") {
		t.Error("goreleaser does not need tag-matches-the-tree, so it publishes before the tag is compared with the tree")
	}
	check := jobBlock(t, workflow, "tag-matches-the-tree")
	for _, f := range []string{"debian/changelog", "internal/shared/version.go", "CHANGELOG.md"} {
		if !strings.Contains(check, f) {
			t.Errorf("tag-matches-the-tree does not read %s", f)
		}
	}
}

// scripts/docs-build.sh builds the site locally in the Ruby docs.yml builds it
// with, and indexes it with the Pagefind the action pins. A Renovate bump moved
// docs.yml's Ruby and left the script three patch releases behind.
func TestTheDocsBuildScriptPinsWhatCIPins(t *testing.T) {
	script := repoFile(t, "scripts", "docs-build.sh")
	pairs := []struct {
		what, ciFile string
		ci, local    *regexp.Regexp
		ciText       string
	}{
		{"Ruby", ".github/workflows/docs.yml",
			regexp.MustCompile(`ruby-version: "([^"]+)"`), regexp.MustCompile(`docker\.io/library/ruby:(\S+)`),
			repoFile(t, ".github", "workflows", "docs.yml")},
		{"Pagefind", ".github/actions/build-search-index/action.yml",
			regexp.MustCompile(`npx --yes pagefind@(\S+)`), regexp.MustCompile(`npx --yes pagefind@(\S+)`),
			repoFile(t, ".github", "actions", "build-search-index", "action.yml")},
	}
	for _, p := range pairs {
		local := p.local.FindStringSubmatch(script)
		ci := p.ci.FindAllStringSubmatch(p.ciText, -1)
		if local == nil || len(ci) == 0 {
			t.Errorf("%s: no pin found (script %v, %s %v)", p.what, local, p.ciFile, ci)
			continue
		}
		for _, c := range ci {
			if c[1] != local[1] {
				t.Errorf("%s: scripts/docs-build.sh pins %s, %s pins %s", p.what, local[1], p.ciFile, c[1])
			}
		}
	}
}

// docs.yml's checks are scripts under scripts/ run through package.json. Its
// paths: filters named neither, so a pull request that changed only a check —
// render-changelog.mjs, prose-check.mjs, docs-check.mjs — ran none of them.
func TestDocsWorkflowRunsWhenItsScriptsChange(t *testing.T) {
	workflow := repoFile(t, ".github", "workflows", "docs.yml")
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(repoFile(t, "package.json")), &pkg); err != nil {
		t.Fatal(err)
	}

	files := map[string]bool{"package.json": true, "package-lock.json": true}
	scriptPath := regexp.MustCompile(`scripts/[\w.-]+`)
	for _, m := range regexp.MustCompile(`npm run ([\w:-]+)`).FindAllStringSubmatch(workflow, -1) {
		cmd, ok := pkg.Scripts[m[1]]
		if !ok {
			t.Errorf("docs.yml runs `npm run %s`, which package.json does not define", m[1])
		}
		for _, f := range scriptPath.FindAllString(cmd, -1) {
			files[f] = true
		}
	}
	if len(files) < 4 {
		t.Fatalf("found %d files docs.yml depends on; this test is reading nothing", len(files))
	}

	on := workflow[strings.Index(workflow, "\non:"):strings.Index(workflow, "\njobs:")]
	push, pr, ok := strings.Cut(on, "\n  pull_request:")
	if !ok {
		t.Fatal("docs.yml's on: has no pull_request trigger")
	}
	for event, block := range map[string]string{"push": push, "pull_request": pr} {
		var globs []string
		for _, g := range regexp.MustCompile(`(?m)^\s+- "([^"]+)"`).FindAllStringSubmatch(block, -1) {
			globs = append(globs, g[1])
		}
		for f := range files {
			hit := false
			for _, g := range globs {
				if g == f || (strings.HasSuffix(g, "/**") && strings.HasPrefix(f, strings.TrimSuffix(g, "**"))) {
					hit = true
				}
			}
			if !hit {
				t.Errorf("docs.yml runs on %s, and its %s paths do not include it: a pull request changing only that file runs no site check", f, event)
			}
		}
	}
}

// A Codecov flag's `paths:` filters what the flag counts. They said internal/
// for the unit upload and internal/core/ for the integration one, so the unit
// run's cmd/ and the integration run's internal/web were uploaded and dropped.
// Every package an uploading `go test` covers has to fall inside its flag's
// paths, when the flag has any.
func TestCodecovFlagsCountWhatTheirUploadCovers(t *testing.T) {
	codecov := repoFile(t, "codecov.yml")
	tests := repoFile(t, ".github", "workflows", "test.yml")

	flags := strings.SplitN(codecov, "\nflags:\n", 2)
	if len(flags) != 2 {
		t.Fatal("codecov.yml has no flags: section")
	}
	paths := map[string][]string{}
	current := ""
	for _, line := range strings.Split(flags[1], "\n") {
		switch {
		case regexp.MustCompile(`^  [a-z]+:$`).MatchString(line):
			current = strings.TrimSuffix(strings.TrimSpace(line), ":")
			paths[current] = nil
		case strings.HasPrefix(line, "      - ") && current != "":
			paths[current] = append(paths[current], strings.TrimSpace(strings.TrimPrefix(line, "      - ")))
		case line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "#"):
			current = ""
		}
	}

	uploads := regexp.MustCompile(`(?s)files: (\S+)\s+flags: (\S+)`).FindAllStringSubmatch(tests, -1)
	if len(uploads) < 2 {
		t.Fatalf("found %d Codecov uploads in test.yml; this test is reading nothing", len(uploads))
	}
	for _, u := range uploads {
		profile, flag := u[1], u[2]
		if _, ok := paths[flag]; !ok {
			t.Errorf("test.yml uploads %s as flag %q, which codecov.yml does not declare", profile, flag)
			continue
		}
		run := regexp.MustCompile(`go test [^\n]*-coverprofile=` + regexp.QuoteMeta(profile) + `[^\n]*`).FindString(tests)
		pkgs := regexp.MustCompile(`\./([\w/]+)/\.\.\.`).FindAllStringSubmatch(run, -1)
		if len(pkgs) == 0 {
			t.Errorf("no `go test` writes %s", profile)
		}
		if len(paths[flag]) == 0 {
			continue // no filter: the flag counts everything uploaded
		}
		for _, p := range pkgs {
			in := false
			for _, prefix := range paths[flag] {
				if strings.HasPrefix(p[1]+"/", strings.TrimSuffix(prefix, "/")+"/") {
					in = true
				}
			}
			if !in {
				t.Errorf("flag %q's paths %v leave out ./%s/..., which its upload covers: Codecov drops it", flag, paths[flag], p[1])
			}
		}
	}
}
