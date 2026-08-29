package shared

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The public demo moved to demo.easywall-project.org, and the move was reported
// as finished. One page had it. Five did not — README.md, both landing-page
// call-to-action buttons, the documentation index table and demo.md — so a
// release later, every route a reader could take to the demo still sent them to
// a host that is not it.
//
// Structural rather than a list of five files, because the list is exactly what
// failed: anything published is checked, and the two files that must keep the
// old host are named here with the reason they keep it.
func TestTheOldDemoHostIsNotPublished(t *testing.T) {
	root := repoRootDir(t)
	const old = "easywall.wdkro.de"

	// Exempt, with the reason. CHANGELOG.md records what 2.4.0 shipped, and
	// rewriting that is a lie about a release. docs/_docs/changelog.md is
	// generated from it by scripts/render-changelog.mjs, so it inherits the
	// same line and the same exemption.
	exempt := map[string]string{
		"CHANGELOG.md":            "records the host 2.4.0 actually announced",
		"docs/_docs/changelog.md": "generated from CHANGELOG.md, which keeps it",
	}

	// docs-tech/ is never published — TestTheTechnicalDocsAreNotPublished holds
	// that structurally — so the specs and plans that record the old host are
	// out of scope by construction rather than by exemption. Anything under a
	// dot-directory (.git, .superpowers, .claude, .serena, .opencode, .github,
	// ...) is tooling or process state, never published, and skipped by name
	// rather than by an enumeration that the next such tool would fall outside
	// of.
	skipDir := map[string]bool{
		"node_modules": true, "vendor": true,
		"docs-tech": true, "bin": true, "_site": true,
	}

	published := map[string]bool{".md": true, ".html": true, ".yml": true,
		".yaml": true, ".json": true, ".toml": true}

	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			if skipDir[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !published[strings.ToLower(filepath.Ext(path))] || exempt[rel] != "" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), old) {
			found = append(found, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the repository: %v", err)
	}

	// The guard has to be able to fail. If both exempt files stopped naming the
	// host, this test would pass by checking nothing meaningful and nobody
	// would know the pattern had stopped matching. And if either exempt file
	// went missing entirely — renamed, deleted — the exemption is dead weight
	// pointing at nothing; catch that too.
	for name := range exempt {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("%s is exempt from this check and does not exist: %v", name, err)
		}
		if !strings.Contains(string(raw), old) {
			t.Errorf("%s no longer contains %q, so its exemption is dead weight — "+
				"delete the entry rather than leaving a rule nothing exercises", name, old)
		}
	}

	sort.Strings(found)
	for _, f := range found {
		t.Errorf("%s names %s, which is not the demo any more\n"+
			"  Liquid pages read site.demo_url from docs/_config.yml; README.md is not "+
			"built by Jekyll and takes the literal", f, old)
	}
}
