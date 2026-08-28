package shared

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// CHANGELOG.md is Keep a Changelog: every version heading has a matching link
// definition at the foot of its section, and `[unreleased]` compares the newest
// release against HEAD. Both halves drift the same way — silently, because a
// missing link definition renders as literal `[2.12.0]` text and a stale
// `[unreleased]` renders as a link that works and points at the wrong diff.
//
// Both had already happened when this test was written. 2.12.0 shipped with no
// link definition at all, and `[unreleased]` still compared against v2.8.0 —
// four releases behind, wrong since 2.9.0, and nothing anywhere said so. The
// release checklist is not where this belongs: the same checklist had been
// followed four times.
//
// This is deliberately not a link *fetcher*. It compares the file against
// itself, so it needs no network and cannot go red because GitHub is slow.
func TestEveryChangelogVersionHasALinkDefinition(t *testing.T) {
	body := changelog(t)

	// `## [2.12.0] — 2026-08-28`, and the older entries' `## [0.0.1] - date`.
	headings := regexp.MustCompile(`(?m)^## \[([0-9][^\]]*)\]`).FindAllStringSubmatch(body, -1)
	if len(headings) == 0 {
		t.Fatal("no version headings found; this test would pass for the wrong reason")
	}

	defined := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\[([0-9][^\]]*)\]:\s`).FindAllStringSubmatch(body, -1) {
		defined[m[1]] = true
	}

	for _, h := range headings {
		if !defined[h[1]] {
			t.Errorf("## [%s] has no `[%s]: …` link definition, so the heading renders "+
				"as literal text instead of a link to its diff", h[1], h[1])
		}
	}
}

// The newest heading is the release `[unreleased]` has to compare against.
// Anything older and the link describes a diff that silently includes releases
// that have already shipped.
func TestUnreleasedComparesAgainstTheNewestRelease(t *testing.T) {
	body := changelog(t)

	newest := regexp.MustCompile(`(?m)^## \[([0-9][^\]]*)\]`).FindStringSubmatch(body)
	if newest == nil {
		t.Fatal("no version heading found; this test would pass for the wrong reason")
	}

	link := regexp.MustCompile(`(?mi)^\[unreleased\]:\s*(\S+)`).FindStringSubmatch(body)
	if link == nil {
		t.Fatal("CHANGELOG.md has no [unreleased] link definition")
	}

	want := "compare/v" + newest[1] + "...HEAD"
	if got := link[1]; !regexp.MustCompile(regexp.QuoteMeta(want) + `$`).MatchString(got) {
		t.Errorf("[unreleased] is %q, but the newest release is %s\n"+
			"  it should end in %q, or it describes a diff that already shipped",
			got, newest[1], want)
	}
}

func changelog(t *testing.T) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), "CHANGELOG.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read CHANGELOG.md: %v", err)
	}
	return string(data)
}
