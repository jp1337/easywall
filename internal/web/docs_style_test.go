package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// docsStylesheet returns the built documentation stylesheet.
func docsStylesheet(t *testing.T) string {
	t.Helper()
	root := filepath.Dir(localesDir(t))
	raw, err := os.ReadFile(filepath.Join(root, "docs", "assets", "css", "style.css"))
	if err != nil {
		t.Fatalf("read built docs stylesheet: %v", err)
	}
	return string(raw)
}

// The documentation site is styled by a stylesheet that is built separately from
// the application's, and nothing else in the test suite looks at it. That gap has
// cost two releases so far: once when removing daisyUI took the page background
// with it and left the docs unreadable in dark mode, and once when a mistyped
// comment terminator swallowed the rule that hides the non-current theme's
// images, so every diagram rendered twice. Both built without an error and both
// were only visible on screen.
//
// These assertions are deliberately about rules whose absence is invisible to a
// build but obvious to a reader.
func TestDocsStylesheetKeepsLoadBearingRules(t *testing.T) {
	css := docsStylesheet(t)

	// Each entry is a rule the documentation cannot do without, and what a
	// reader sees when it goes missing.
	required := []struct{ pattern, breaks string }{
		{`\.themed-img\{display:none\}`,
			"both the light and the dark copy of every diagram render, stacked"},
		{`\[data-theme=easywall-dark\] \.themed-img-dark\{display:block\}`,
			"no diagram or screenshot renders at all in dark mode"},
		{`html,body\{[^}]*background:var\(--bg\)`,
			"the page has no background of its own — dark mode renders on white"},
		{`\.content-body ul\{list-style-type:disc\}`,
			"Tailwind's preflight wins and every bullet list loses its markers"},
		{`\.content-body ol\{list-style-type:decimal\}`,
			"numbered lists lose the numbers, including the priority order in configuration.md"},
	}

	for _, r := range required {
		if !regexp.MustCompile(r.pattern).MatchString(css) {
			t.Errorf("docs/assets/css/style.css is missing %s\n  without it: %s\n"+
				"  rebuild with `npm run build:docs-css`, and check web/src/docs.css "+
				"for an unterminated comment — that deletes rules silently",
				r.pattern, r.breaks)
		}
	}
}

// Exactly one element may carry the frame around a code block. kramdown nests
// div.highlighter-rouge > div.highlight > pre.highlight, so a background or a
// border on more than one of them draws a box inside a box. That shipped, and
// went unnoticed because the border colour it used was near-invisible.
func TestDocsCodeBlockHasASingleFrame(t *testing.T) {
	css := docsStylesheet(t)

	framed := regexp.MustCompile(`\.content-body div\.highlight[^{,]*\{[^}]*border:1px`)
	if m := framed.FindString(css); m != "" && !strings.Contains(m, "highlighter-rouge") {
		t.Errorf("div.highlight carries a border of its own: %q\n"+
			"  the frame belongs to .content-body div.highlighter-rouge alone, "+
			"otherwise every highlighted block renders one box inside another", m)
	}

	reset := `\.content-body div\.highlighter-rouge div\.highlight,` +
		`\.content-body div\.highlighter-rouge pre\.highlight\{background:0 0;border:0`
	if !regexp.MustCompile(reset).MatchString(css) {
		t.Error("the inner-element reset for highlighted code blocks is missing; " +
			"div.highlight and pre.highlight must both be bare inside the frame")
	}
}

// The inline-code chip must not follow <code> into a <pre>. A theme-scoped copy
// of the base rule used to outrank the reset on specificity, and because <code>
// is an inline box the chip fill painted per line box — ragged grey rectangles
// behind parts of every code block, in light mode only.
func TestDocsInlineCodeIsNotThemeScoped(t *testing.T) {
	css := docsStylesheet(t)

	bad := regexp.MustCompile(`\[data-theme=[^\]]+\] \.content-body code\{`)
	if m := bad.FindString(css); m != "" {
		t.Errorf("found a theme-scoped rule on .content-body code: %q\n"+
			"  it outranks `.content-body pre code` (0,2,2 vs 0,3,1) and leaks the "+
			"inline chip background into code blocks; the colours already come from "+
			"custom properties, so the theme scope buys nothing", m)
	}
}

// The documentation sidebar shows a version badge. It was hardcoded in the
// layout, so it silently drifted a patch release behind what was published.
// It now comes from docs/_config.yml, and this keeps that value honest.
func TestDocsVersionMatchesRelease(t *testing.T) {
	root := filepath.Dir(localesDir(t))

	cfg, err := os.ReadFile(filepath.Join(root, "docs", "_config.yml"))
	if err != nil {
		t.Fatalf("read docs/_config.yml: %v", err)
	}
	m := regexp.MustCompile(`(?m)^version:\s*"([^"]+)"`).FindSubmatch(cfg)
	if m == nil {
		t.Fatal("docs/_config.yml has no top-level version key; the sidebar badge reads it")
	}
	if got := string(m[1]); got != shared.CurrentVersion {
		t.Errorf("docs/_config.yml says version %q, shared.CurrentVersion is %q\n"+
			"  the sidebar badge on every documentation page would show the wrong release",
			got, shared.CurrentVersion)
	}

	// And the layout must still read it rather than spelling a version out.
	layout, err := os.ReadFile(filepath.Join(root, "docs", "_layouts", "default.html"))
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	if !strings.Contains(string(layout), `{{ site.version }}`) {
		t.Error("the sidebar version badge no longer reads site.version; " +
			"a literal there cannot be kept in step with a release")
	}
}
