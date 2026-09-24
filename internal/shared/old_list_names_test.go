package shared

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The two lists have been the blocklist and the allowlist since 2.23. The old
// words stay only where something written before 2.23 has to keep being read,
// and in history.
//
// Counted exactly, per file: a new occurrence fails, and so does a missing
// one — the counts below are the migration, and a count that drops means a
// reader of the old name was deleted and an upgrade from 2.22 stops working.
// When one changes on purpose, change it here and say why in the commit.
//
// The pattern is assembled rather than written out, so it does not count
// itself; the paths and comments in this file do, and are listed like any
// other.
func TestTheOldListNamesAreGone(t *testing.T) {
	root := repoRoot(t)
	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}
	old := regexp.MustCompile(`(?i)` + "black" + "list|" + "white" + "list")

	// History: what was true when it was written.
	history := func(name string) bool {
		switch name {
		case "CHANGELOG.md", "docs/_docs/changelog.md", "debian/changelog":
			return true
		}
		return strings.HasPrefix(name, "docs-tech/plans/") || strings.HasPrefix(name, "docs-tech/specs/") ||
			// Files the v2.22.0 code wrote, kept verbatim as upgrade fixtures.
			strings.HasPrefix(name, "internal/shared/testdata/upgrade-2.22/")
	}

	// file → occurrences, each one a reader of something 2.22 wrote, or an
	// upstream name.
	allowed := map[string]int{
		"internal/shared/renamed.go":            8,  // the name map; Rules.UnmarshalJSON's two old keys, twice each
		"internal/shared/packetlog.go":          1,  // RuleFromPrefix: a rule 2.22 loaded keeps its prefix until the next apply
		"internal/core/config.go":               10, // readOldLogKeys: the two TOML keys, their tags and the warning
		"internal/core/appliedconfig.go":        5,  // applied-config.json from 2.22 carries LogBlacklist(Limit)
		"internal/core/packetlog.go":            1,  // packets.log replay
		"internal/web/server.go":                4,  // GET 301 and POST of /blacklist and /whitelist
		"internal/web/handler_blocked.go":       1,  // a saved /blocked?rule= filter and a stale row action
		"docs/_docs/features/blocklist.md":      1,  // redirect_from: the old URL
		"docs/_docs/features/allowlist.md":      1,  // redirect_from: the old URL
		"internal/web/acme.go":                  1,  // autocert.HostWhitelist, golang.org/x/crypto's name
		"internal/web/acme_integration_test.go": 1,  // the same
		"docs-tech/invariants.md":               1,  // the same
		// The tests that prove the readers above.
		"internal/shared/renamed_test.go":        7,
		"internal/shared/docs_coverage_test.go":  2,
		"internal/core/upgrade_222_test.go":      8,
		"internal/web/renamed_routes_test.go":    14,
		"internal/shared/old_list_names_test.go": 6, // this file: the paths and comments above
	}
	// Paths that keep an old word in their name. The stub answers the old URL
	// /features/blacklist/; renamed, that URL is a 404.
	allowedPaths := map[string]bool{"docs/features/blacklist.md": true}

	got := map[string]int{}
	for _, name := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if name == "" || history(name) {
			continue
		}
		if old.MatchString(name) && !allowedPaths[name] {
			t.Errorf("%s: the file name uses the pre-2.23 list name", name)
		}
		if skipPath(name) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, name)) // #nosec G304 -- a path git listed in this repository
		if err != nil {
			continue
		}
		if n := len(old.FindAllIndex(data, -1)); n > 0 {
			got[name] = n
		}
	}

	names := make([]string, 0, len(got)+len(allowed))
	for n := range got {
		names = append(names, n)
	}
	for n := range allowed {
		if _, ok := got[n]; !ok {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		switch g, w := got[n], allowed[n]; {
		case g > w:
			t.Errorf("%s: %d occurrences of the pre-2.23 list names, %d allowed — "+
				"write blocklist/allowlist, or, for a reader of a 2.22 file, raise the count here", n, g, w)
		case g < w:
			t.Errorf("%s: %d occurrences of the pre-2.23 list names, %d expected — "+
				"a reader of something 2.22 wrote may be gone; lower the count here only if that is deliberate", n, g, w)
		}
	}
}
