package web

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
		// The gutter has to be reserved separately from the column's width.
		// --toc-w was doing both jobs, so the article's right edge landed exactly
		// on the contents' border-left: a hairline against the text, and a figure
		// running flush into it. Nothing failed — the contents column was the
		// right width, and the reserved space was the right width, and they were
		// the same number.
		{`padding-right:calc\(var\(--content-pad\) \+ var\(--toc-w\) \+ var\(--toc-gap\)\)`,
			"the prose runs straight into the on-page contents with no gutter between them"},
		// `.sr-only` used to be asserted here. It only ever existed because the
		// search field carried a visually hidden <label> and Tailwind's @source
		// scan reached the include. The trigger that replaced that field has
		// visible text and an aria-label, so nothing under docs/ uses the class
		// any more and the utility is correctly absent from the built file.
		// Asserting it now would demand a rule for markup that no longer exists.
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

// The audit log page lists which actions carry colour, and the stylesheet-level
// mapping lives in auditActionTones. The two drifted once already: the table
// said four while the code coloured five, and the one it omitted was
// rollback_failed — the entry the same page calls the one worth alerting on.
func TestAuditColourTableMatchesTheCode(t *testing.T) {
	root := filepath.Dir(localesDir(t))
	raw, err := os.ReadFile(filepath.Join(root, "docs", "_docs", "features", "audit-log.md"))
	if err != nil {
		t.Fatalf("read audit-log.md: %v", err)
	}
	docs := string(raw)

	for action := range auditActionTones {
		if !strings.Contains(docs, "`"+action+"`") {
			t.Errorf("audit-log.md does not list %q, which the interface colours", action)
		}
	}

	// And the count in the heading has to match.
	want := fmt.Sprintf("Only %s entries carry colour", numberWord(len(auditActionTones)))
	if !strings.Contains(docs, want) {
		t.Errorf("the heading should read %q — there are %d coloured actions",
			want, len(auditActionTones))
	}
}

// numberWord spells the small numbers the heading uses.
func numberWord(n int) string {
	words := []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight"}
	if n < len(words) {
		return words[n]
	}
	return fmt.Sprint(n)
}

// The mobile drawer's backdrop (.sidebar-backdrop) and the drawer itself
// (.sidebar) are position:fixed siblings, so their z-index alone decides which
// one receives a tap. Since cd89c02d (2026-05-03) the backdrop outranked the
// sidebar even while it carried .open — every tap inside an opened drawer,
// on any nav link or the search field, landed on the backdrop instead and
// only ever closed it. A phone's hit-testing is invisible to `go build` and
// to `npm run build:docs-css`; this at least keeps the ordering itself honest.
func TestMobileSidebarOutranksItsBackdrop(t *testing.T) {
	css := docsStylesheet(t)

	backdrop := regexp.MustCompile(`\.sidebar-backdrop\{[^}]*z-index:(\d+)`).FindStringSubmatch(css)
	if backdrop == nil {
		t.Fatal("no z-index found on .sidebar-backdrop in docs/assets/css/style.css")
	}
	open := regexp.MustCompile(`\.sidebar\.open\{[^}]*z-index:(\d+)`).FindStringSubmatch(css)
	if open == nil {
		t.Fatal("no z-index found on .sidebar.open in docs/assets/css/style.css")
	}

	backdropZ, err := strconv.Atoi(backdrop[1])
	if err != nil {
		t.Fatalf("parse .sidebar-backdrop z-index %q: %v", backdrop[1], err)
	}
	openZ, err := strconv.Atoi(open[1])
	if err != nil {
		t.Fatalf("parse .sidebar.open z-index %q: %v", open[1], err)
	}

	if openZ <= backdropZ {
		t.Errorf(".sidebar.open z-index (%d) does not outrank .sidebar-backdrop (%d)\n"+
			"  an open drawer would sit behind its own backdrop, and nothing inside it — "+
			"no nav link, no search field — could receive a tap on a phone",
			openZ, backdropZ)
	}
}

// The search panel's rules override Pagefind's own class names, and they only
// work from outside a cascade layer. Pagefind's stylesheet is fetched at runtime
// and is unlayered; an unlayered declaration beats every declaration in a named
// layer no matter how specific it is. Written first inside @layer components,
// every one of these rules lost to Pagefind's own — the overlay rendered a
// yellow <mark> and a white input on a dark panel, and nothing failed: the build
// was green, the rules were present in the built file, and the grep for them
// passed. Only the browser said otherwise.
//
// So this asserts placement, not presence: the rules exist AND they are not
// inside the layer.
func TestTheSearchOverridesAreOutsideTheCascadeLayer(t *testing.T) {
	css := docsStylesheet(t)

	const layerOpen = "@layer components{"
	start := strings.Index(css, layerOpen)
	if start < 0 {
		t.Fatalf("the built docs stylesheet has no %q, so this test cannot tell "+
			"layered rules from unlayered ones any more", layerOpen)
	}

	depth, end := 0, -1
	for i := start + len(layerOpen) - 1; i < len(css); i++ {
		switch css[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		t.Fatal("the @layer components block in the built docs stylesheet never closes")
	}
	layer := css[start:end]

	// One per override that stops working when it is layered. The <mark> pair is
	// the loudest of them: without it the browser default applies, and #ffff00
	// is what --state-warn means in this interface.
	for _, sel := range []string{
		"#docs-search-panel .pagefind-ui__search-input",
		"#docs-search-panel .pagefind-ui__drawer",
		"#docs-search-panel .pagefind-ui__result-excerpt",
		"#docs-search-panel mark",
		"article.content-body mark.pagefind-highlight",
	} {
		if !strings.Contains(css, sel) {
			t.Errorf("the built docs stylesheet has no rule for %q at all", sel)
			continue
		}
		if strings.Contains(layer, sel) {
			t.Errorf("%q sits inside @layer components in the built docs stylesheet — "+
				"Pagefind's own unlayered rule wins over it, so the declaration ships and "+
				"does nothing", sel)
		}
	}
}
