package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// stylesheetPaths are the two sources and the two committed build outputs.
//
// Both halves, because they can disagree: Tailwind drops rules silently, so a
// token present in web/src and absent from the built file is a different defect
// from one present in both — and only the built files are what ships.
func stylesheetPaths(t *testing.T) []string {
	t.Helper()
	root := filepath.Dir(localesDir(t))
	return []string{
		filepath.Join(root, "web", "src", "app.css"),
		filepath.Join(root, "web", "src", "docs.css"),
		filepath.Join(root, "web", "static", "style.css"),
		filepath.Join(root, "docs", "assets", "css", "style.css"),
	}
}

// No accent token survives, in either stylesheet.
//
// 2.16 removed the ice-blue accent because a page carrying it in fourteen places
// has no accent at all — /settings carried exactly fourteen. What this guards is
// not the removal but the return: a token reintroduced "just for this one case"
// six months from now is a one-line diff nobody reads twice, and it puts colour
// back into a system whose entire remaining rule is that colour means firewall
// state.
//
// The custom properties, not the word. `.docs-hero-accent` is a class name that
// 2.16 renames on its own account; matching a bare "accent" would fail on markup
// rather than on colour, which is not what this is about. --code-keyword is
// deliberately untouched and deliberately still #8fd3fb: syntax highlighting is
// its own semantic domain, a documentation page carries no firewall state for
// the hue to collide with, and matching on the hexadecimal rather than the name
// would take it with the rest.
func TestNoAccentTokenSurvives(t *testing.T) {
	token := regexp.MustCompile(`--(color-)?accent[a-z-]*\b`)

	for _, path := range stylesheetPaths(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		name := filepath.Base(filepath.Dir(path)) + "/" + filepath.Base(path)

		for i, line := range strings.Split(string(raw), "\n") {
			// A comment may name the token it replaced — that is how the
			// reasoning survives. A declaration may not.
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") ||
				strings.HasPrefix(trimmed, "//") {
				continue
			}
			if m := token.FindString(line); m != "" {
				t.Errorf("%s:%d still names %q\n"+
					"  colour means firewall state after 2.16; focus, selection and the "+
					"primary action are carried by fill, edge and weight", name, i+1, m)
			}
		}
	}
}

// And the literal hues a token rename walks straight past.
//
// docs.css hard-codes three colours from palettes that were retired before this
// release: the Aurora cyan and its light-mode twin on .sidebar-version,
// .docs-hero-pill and .docs-design-row-num, and a teal in both hero glows. They
// are not tokens, so TestNoAccentTokenSurvives cannot see them, and they are the
// reason "no accent token survives" is necessary and not sufficient.
func TestNoRetiredHueSurvives(t *testing.T) {
	retired := []struct{ literal, was string }{
		{"34,211,238", "the retired Aurora cyan"},
		{"8,145,178", "its light-mode twin"},
		{"45,212,191", "the teal in the hero glows"},
	}

	for _, path := range stylesheetPaths(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		name := filepath.Base(filepath.Dir(path)) + "/" + filepath.Base(path)
		body := strings.ReplaceAll(string(raw), " ", "")

		for _, hue := range retired {
			if strings.Contains(body, hue.literal) {
				t.Errorf("%s still carries rgba(%s,…) — %s\n"+
					"  a grep over token names cannot see a literal; this is the half "+
					"that keeps the palette honest", name, hue.literal, hue.was)
			}
		}
	}
}
