package web

import (
	"os"
	"path/filepath"
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
	layout := docsLayout(t)

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
	if !strings.Contains(css, "#docs-search{display:none}") &&
		!strings.Contains(css, "#docs-search {display: none}") {
		t.Error("the built docs stylesheet does not hide #docs-search by default; " +
			"without a script the page would show a search field that cannot search")
	}
	if !strings.Contains(css, `[data-js] #docs-search`) {
		t.Error("nothing in the built docs stylesheet reveals #docs-search once data-js is set")
	}
}
