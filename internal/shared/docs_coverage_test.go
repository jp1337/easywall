package shared

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every page an operator can open has a page in the documentation that describes
// it.
//
// This is the same idea as TestEveryConfigKeyIsDocumented, applied to the other
// half of the surface: that test derives its list from the config structs, this
// one from the router. A page nobody documented is a feature an operator can only
// understand by clicking around it and guessing.
//
// It was three pages when this was written, and one of them mattered: /firstrun
// is the first thing every single installation shows, it decides the SSH port,
// IPv6 and whether the host is counted, and it deliberately stages everything but
// the account — behaviour that reads as a bug when nothing explains it. /apply is
// the feature easywall exists for, and its screenshots had been sitting in
// docs/assets/img/screens/ referenced by nothing at all.
//
// A new route therefore has to be answered here: name the page that documents it,
// or say why it is not a page. Both are cheap; neither happens by itself.
func TestEveryPageIsDocumented(t *testing.T) {
	// route → the page that describes it, relative to docs/.
	documented := map[string]string{
		"/login":      "installation/first-run.md",
		"/firstrun":   "installation/first-run.md",
		"/password":   "installation/first-run.md",
		"/dashboard":  "features/dashboard.md",
		"/apply":      "features/apply.md",
		"/ports":      "features/ports.md",
		"/blacklist":  "features/blacklist.md",
		"/whitelist":  "features/blacklist.md",
		"/forwarding": "features/forwarding.md",
		"/custom":     "features/custom-rules.md",
		"/options":    "features/filters.md",
		"/settings":   "features/system-settings.md",
		"/system":     "features/system-settings.md",
		"/log":        "features/audit-log.md",
		"/export":     "features/export-import.md",
	}

	// Not pages: redirects, polling endpoints, fragments answered into a page
	// that is already documented, and the asset handler. Each one is listed by
	// name rather than matched by a pattern, so a new route cannot slip in by
	// happening to look like one of these.
	notAPage := map[string]string{
		"/":              "redirects to /dashboard",
		"/logout":        "an action, not a page — the behaviour is in security.md",
		"/apply/status":  "polled by the apply page for the countdown",
		"/log/filter":    "an htmx fragment of the audit log page",
		"/static/*":      "the asset handler",
		"/schemas/*":     "the JSON schemas, served as files",
		"/healthz":       "a probe endpoint, deliberately not in the operator docs",
		"/favicon.ico":   "an asset",
		"/robots.txt":    "an asset",
		"/.well-known/*": "an asset",
	}

	root := repoRootDir(t)
	server := repoFile(t, "internal", "web", "server.go")

	// Only GET: a POST route is the same page answering its own form, and the
	// pages that accept one all have a GET beside it.
	//
	// The word boundary is load-bearing: without it `r.Header.Get("HX-Request")`
	// matches, because "Header." ends in the same two characters the router does.
	routes := regexp.MustCompile(`\br\.(?:Get|Handle)\("([^"]+)"`).FindAllStringSubmatch(server, -1)
	if len(routes) < 10 {
		t.Fatalf("found %d routes in server.go; the pattern no longer matches how they "+
			"are registered, so this test would pass by finding nothing", len(routes))
	}

	for _, m := range routes {
		route := m[1]
		if reason, ok := notAPage[route]; ok {
			_ = reason
			continue
		}
		page, ok := documented[route]
		if !ok {
			t.Errorf("%s is served to operators and this test does not know a page for it\n"+
				"  add it to `documented` with the page that describes it, or to `notAPage` with the reason",
				route)
			continue
		}
		if _, err := os.Stat(filepath.Join(root, "docs", page)); err != nil {
			t.Errorf("%s is documented in docs/%s, which does not exist", route, page)
		}
	}
}

// The technical documentation is written for whoever maintains this repository,
// and it is not published.
//
// It says which incident produced which rule, what the release actually does, and
// where the packaging has bitten before. That belongs in the repository and not on
// easywall-project.org — and the way to be sure of it is structural rather than a
// list: Jekyll builds from docs/ and only from docs/, so a directory outside docs/
// cannot be published even if someone forgets an exclude entry.
//
// This test holds both halves of that: the files are outside docs/, and the
// workflow still builds from docs/. Moving the technical docs under docs/, or
// pointing the build at the repository root, fails here.
func TestTheTechnicalDocsAreNotPublished(t *testing.T) {
	root := repoRootDir(t)

	entries, err := os.ReadDir(filepath.Join(root, "docs-tech"))
	if err != nil {
		t.Fatalf("docs-tech/ is missing: %v\n"+
			"  it is where the maintainer-facing documentation lives, deliberately "+
			"outside the Jekyll source", err)
	}
	if len(entries) == 0 {
		t.Fatal("docs-tech/ is empty")
	}

	// Nothing of it may sit inside the published tree, under any name.
	for _, name := range []string{"docs-tech", "technical", "internal"} {
		if _, err := os.Stat(filepath.Join(root, "docs", name)); err == nil {
			t.Errorf("docs/%s exists — anything under docs/ is built and published by "+
				"docs.yml, which is what this directory is not for", name)
		}
	}

	// And the build still reads only docs/. Two jobs, both with the same
	// working directory; a third one, or a changed path, is a change of scope
	// that has to be noticed here.
	wf := repoFile(t, ".github", "workflows", "docs.yml")
	builds := regexp.MustCompile(`(?m)^\s+run:\s+bundle exec jekyll build`).FindAllString(wf, -1)
	dirs := regexp.MustCompile(`(?m)^\s+working-directory:\s+(\S+)`).FindAllStringSubmatch(wf, -1)
	if len(builds) == 0 {
		t.Fatal("docs.yml no longer runs `bundle exec jekyll build`; this test cannot " +
			"tell what gets published any more")
	}
	for _, d := range dirs {
		if strings.Trim(d[1], `"'`) != "docs" {
			t.Errorf("docs.yml builds from %q; only docs/ may be published, or the "+
				"technical documentation goes online with it", d[1])
		}
	}
	if len(dirs) < len(builds) {
		t.Errorf("%d jekyll build steps but only %d working-directory entries — a build "+
			"without one runs from the repository root and would publish everything",
			len(builds), len(dirs))
	}
}

// Every command the protocol declares must be documented in both the operator
// documentation and the technical documentation. This catches drifts like the
// one where PANIC was added to the constants and the architecture table but
// nothing told the next person to do that: now, adding a command and forgetting
// a table is caught immediately.
//
// The list is derived from AllCommandTypes, which is published by the protocol
// itself, so this test catches failures at the source.
func TestEveryCommandIsDocumentedInBothPublishedAndTechnicalDocs(t *testing.T) {
	archDocs := repoFile(t, "docs", "architecture.md")
	techDocs := repoFile(t, "docs-tech", "protocol.md")

	if len(AllCommandTypes) == 0 {
		t.Fatal("AllCommandTypes is empty; the list has not been populated or has been broken")
	}

	for _, cmd := range AllCommandTypes {
		cmdStr := "`" + string(cmd) + "`"
		if !strings.Contains(archDocs, cmdStr) {
			t.Errorf("docs/architecture.md does not document command %s", cmdStr)
		}
		if !strings.Contains(techDocs, cmdStr) {
			t.Errorf("docs-tech/protocol.md does not document command %s", cmdStr)
		}
	}
}
