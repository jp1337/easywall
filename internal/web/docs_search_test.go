package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docsLayout returns docs/_layouts/default.html.
func docsLayout(t *testing.T) string {
	t.Helper()
	root := filepath.Dir(localesDir(t))
	raw, err := os.ReadFile(filepath.Join(root, "docs", "_layouts", "default.html"))
	if err != nil {
		t.Fatalf("read the docs layout: %v", err)
	}
	return string(raw)
}

// The search is a container the layout renders and a script mounts into. Either
// half can go missing without anything failing: the page still renders, there is
// simply no search, and a stylesheet diff cannot see it.
func TestTheDocsSidebarRendersTheSearchContainer(t *testing.T) {
	layout := docsLayoutCode(t)

	for _, want := range []struct{ needle, why string }{
		{`{% include search.html %}`, "the sidebar must include the search partial"},
		{"data-js", "the layout must announce that a script is running, or the field cannot be hidden when one is not"},
	} {
		if !strings.Contains(layout, want.needle) {
			t.Errorf("docs/_layouts/default.html has no %q — %s", want.needle, want.why)
		}
	}

	root := filepath.Dir(localesDir(t))
	raw, err := os.ReadFile(filepath.Join(root, "docs", "_includes", "search.html"))
	if err != nil {
		t.Fatalf("read the search include: %v", err)
	}
	if !strings.Contains(string(raw), `id="docs-search"`) {
		t.Error(`docs/_includes/search.html has no id="docs-search" — PagefindUI mounts by that id`)
	}
}

// A field that cannot search is worse than no field. Without JavaScript there is
// no index to query and no way to get one, so the control is hidden and the
// sidebar navigation stays the complete scriptless path to every page.
//
// Asserted against the *built* stylesheet, because Tailwind is what decides
// whether the rule survives, and it has dropped one before.
func TestTheSearchFieldIsHiddenWithoutJavaScript(t *testing.T) {
	css := docsStylesheet(t)
	// The declaration, not the whole rule text. It was matched as the exact
	// string "#docs-search{display:none}", which held only while that selector
	// carried nothing else — merging Pagefind's theme variables into the same
	// block broke the assertion without touching what it was asserting.
	if !regexp.MustCompile(`#docs-search\{[^}]*display:none`).MatchString(css) {
		t.Error("the built docs stylesheet does not hide #docs-search by default; " +
			"without a script the page would show a search field that cannot search")
	}
	if !regexp.MustCompile(`\[data-js\] #docs-search\{[^}]*display:block`).MatchString(css) {
		t.Error("nothing in the built docs stylesheet reveals #docs-search once data-js is set")
	}
}

// docsLayoutCode is the layout with everything a browser never runs removed:
// Liquid comment blocks and JavaScript line comments.
//
// Reading the file whole was not enough. The first version of the test below
// searched for the bare string "data-js", which the comment above the theme
// script also contains — so the assertion would have passed on a layout that
// only *talked* about the flag. The same trap is in workflow_order_test.go twice,
// where matching an action's name found the comment naming it instead of the
// `uses:` line. Prose is not behaviour.
func docsLayoutCode(t *testing.T) string {
	t.Helper()
	code := regexp.MustCompile(`(?s)\{%-?\s*comment\s*-?%\}.*?\{%-?\s*endcomment\s*-?%\}`).
		ReplaceAllString(docsLayout(t), "")
	var kept []string
	for _, line := range strings.Split(code, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// The loader is the feature, and nothing was looking at it.
//
// The container and the stylesheet rule are guarded above; the ~50 lines that
// fetch Pagefind and mount it were not, and every one of those guards passed
// with the whole <script> block deleted. What is left then is a placeholder
// input that takes a keystroke and does nothing for ever — a control that lies
// about what it can do, which is the one outcome the design set out to avoid.
//
// The third assertion carries its own incident. pagefind-highlight.js 1.5.2 is
// an ES module whose body also assigns window.PagefindHighlight; attached as a
// classic script it throws on the `export` before that assignment is ever
// reached, so window.PagefindHighlight stays undefined, `new` on it throws, and
// a visitor arriving from a search result simply sees no term marked. Nothing in
// the browser says anything. It was found by loading the real bundle, and this is
// what keeps `type = 'module'` from being tidied away by somebody who reads the
// two lines and sees a redundant property.
func TestTheDocsLayoutMountsPagefind(t *testing.T) {
	code := docsLayoutCode(t)

	for _, want := range []struct{ needle, why string }{
		{"pagefind-ui.js",
			"nothing fetches the search bundle, so the placeholder input never becomes a search field"},
		{"new window.PagefindUI(",
			"the bundle is fetched and never mounted"},
		{"element: '#docs-search'",
			"PagefindUI is not pointed at the container the sidebar renders, so it mounts nowhere"},
	} {
		if !strings.Contains(code, want.needle) {
			t.Errorf("docs/_layouts/default.html has no %q — %s", want.needle, want.why)
		}
	}

	// Tied to the element that loads the highlight file rather than searched for
	// loose, so renaming the variable cannot leave this passing on nothing.
	m := regexp.MustCompile(`(?m)^\s*(\w+)\.src\s*=\s*'[^']*pagefind-highlight\.js`).
		FindStringSubmatch(code)
	if m == nil {
		t.Fatal("nothing in docs/_layouts/default.html loads pagefind-highlight.js; " +
			"a visitor arriving from a search result lands on the page with the term unmarked")
	}
	if !regexp.MustCompile(regexp.QuoteMeta(m[1]) + `\.type\s*=\s*'module'`).MatchString(code) {
		t.Errorf("the script element loading pagefind-highlight.js (%s) is not given "+
			"type = 'module'\n"+
			"  that file is an ES module whose body also sets window.PagefindHighlight. "+
			"As a classic script it fails to parse on its `export`, the global is never "+
			"assigned, and `new window.PagefindHighlight(...)` throws — highlighting is "+
			"silently dead and the browser reports nothing a visitor would see", m[1])
	}
}
