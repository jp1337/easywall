package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// appJSSource returns web/static/app.js.
func appJSSource(t *testing.T) string {
	t.Helper()
	root := filepath.Dir(localesDir(t))
	raw, err := os.ReadFile(filepath.Join(root, "web", "static", "app.js"))
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	return string(raw)
}

// The ports form rebuilds the entire rule list out of the DOM on every save —
// syncHidden reads each row's fields and JSON.stringifies the lot. A field the
// row does not carry is a field the save drops.
//
// So a rule id that is rendered into the page and not read back out again means
// every save re-keys every rule on the page: EnsureRuleIDs sees an empty id,
// mints a new one, and every usage counter that rule had is orphaned. Editing
// one description would silently reset the history of all of them. The column
// would still render, which is what makes this worth a guard rather than a
// comment.
func TestThePortsFormCarriesTheRuleIDBothWays(t *testing.T) {
	tmpl, err := os.ReadFile(filepath.Join(templatesDir(t), "ports.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tmpl), `data-id="{{$r.ID}}"`) {
		t.Error(`ports.html does not render data-id="{{$r.ID}}" on the rule row; ` +
			"the id never reaches the browser, so every save mints a new one")
	}

	js := appJSSource(t)
	if !strings.Contains(js, "tr.dataset.id") {
		t.Error("app.js never reads tr.dataset.id; syncHidden rebuilds the rule list " +
			"from the DOM, so an id it does not read is an id the save drops")
	}
	if !strings.Contains(js, "rule.id = id") {
		t.Error("app.js reads the id but does not put it on the rule it submits")
	}
}
