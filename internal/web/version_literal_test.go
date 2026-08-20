package web

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// SVG path data uses `v` for a vertical lineto ("v18", "v-2.25"), so the whole
// element is removed before the search. Verified against the templates as they
// stand: with the elements stripped, the pattern finds exactly the three
// literals this test exists to remove and nothing else.
var svgElementRe = regexp.MustCompile(`(?s)<svg\b.*?</svg>`)

// A version literal: v2, v2.8, v2.8.0.
var versionLiteralRe = regexp.MustCompile(`\bv[0-9]+(\.[0-9]+)*\b`)

// web/templates/base.html carried the literal `v2` — not the version, the major.
// It would have read `v2` in 2.5 and would read it in 2.19, and the two paths by
// which shared.CurrentVersion already reached the templates showed it nowhere:
// server.go uses it as an asset cache-buster, and dashboard.html wrapped
// .Version in {{if .Version.UpdateAvailable}}, which names the *new* version and
// never the installed one. Whoever was current saw nothing; whoever was not
// learned where to go but not where they were.
//
// This is the half that keeps it from coming back.
func TestNoTemplateCarriesAVersionLiteral(t *testing.T) {
	dir := repoTemplates(t)
	files, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no templates found in %s (err %v); this test would pass by finding nothing", dir, err)
	}
	for _, f := range files {
		data, err := os.ReadFile(f) // #nosec G304 -- a path from Glob over the repository's own template dir
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		stripped := svgElementRe.ReplaceAllString(string(data), "")
		for _, m := range versionLiteralRe.FindAllString(stripped, -1) {
			t.Errorf("%s carries the version literal %q; render it from PageData.Version, "+
				"which -ldflags -X actually writes to", filepath.Base(f), m)
		}
	}
}
