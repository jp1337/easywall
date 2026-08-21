package web

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// reviewListIDRe matches the id cell of a table row in docs-tech/i18n-review.md,
// e.g. "| `blacklist_subtitle` | Sources that are... |". Anchored to the start
// of the line so a backtick elsewhere in the row — the English text column
// quotes `password`, `totp_secret` and the like — is never mistaken for an id.
var reviewListIDRe = regexp.MustCompile("(?m)^\\|\\s*`([a-z][a-z0-9_]*)`\\s*\\|")

// reviewListIDs parses docs-tech/i18n-review.md and returns every id it names,
// in the order they appear. The list is derived from the document rather than
// hand-copied here, so the test and the document cannot silently diverge.
func reviewListIDs(t *testing.T) []string {
	t.Helper()
	root := filepath.Dir(localesDir(t))
	path := filepath.Join(root, "docs-tech", "i18n-review.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	matches := reviewListIDRe.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatalf("found no ids in %s — the regex or the document's table format changed", path)
	}
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		ids = append(ids, m[1])
	}
	return ids
}

// A reviewer's list that names a key en.json no longer has sends them hunting
// for text that is not there. Every id docs-tech/i18n-review.md names must be
// a real key, so a rename or removal in en.json is caught here rather than by
// a confused translator.
func TestReviewListNamesRealKeys(t *testing.T) {
	ids := reviewListIDs(t)
	en := localeIDs(t, "en")

	var missing []string
	for _, id := range ids {
		if !en[id] {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	for _, id := range missing {
		t.Errorf("docs-tech/i18n-review.md names %q, which is not a key in locales/en.json", id)
	}
}
