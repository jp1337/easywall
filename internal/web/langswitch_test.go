package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// langswitchBlock returns the body of {{define "langswitch"}} from base.html.
func langswitchBlock(t *testing.T) string {
	t.Helper()
	root := filepath.Dir(localesDir(t)) // localesDir walks up to <repo>/locales
	raw, err := os.ReadFile(filepath.Join(root, "web", "templates", "base.html"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	start := strings.Index(body, `{{define "langswitch"}}`)
	if start < 0 {
		t.Fatal(`base.html has no {{define "langswitch"}}`)
	}
	rest := body[start:]
	end := strings.Index(rest, "{{end}}\n{{end}}")
	if end < 0 {
		t.Fatal("could not find the end of the langswitch block")
	}
	return rest[:end]
}

// The switcher has to work with JavaScript switched off. The reason is written
// above {{define "langswitch"}} and predates this change: an operator who
// cannot read the interface should not also need JavaScript to fix that.
func TestLangSwitch_WorksWithoutJavaScript(t *testing.T) {
	block := langswitchBlock(t)

	for _, want := range []struct{ needle, why string }{
		{"<select", "the switcher must be a select; chip buttons do not hold eleven languages"},
		{`name="lang"`, "the select must post the lang field the handler reads"},
		{`type="submit"`, "a submit button must be in the markup, not added by a script"},
		{"lang-submit", "the submit button needs the class the stylesheet hides when JS is on"},
		{`method="POST"`, "the form writes a cookie, so it must not be a GET"},
	} {
		if !strings.Contains(block, want.needle) {
			t.Errorf("langswitch has no %q — %s", want.needle, want.why)
		}
	}

	// The old chip markup must be gone, or both render and the sidebar grows.
	if strings.Contains(block, "lang-option") && !strings.Contains(block, "lang-options-legacy") {
		t.Error("the chip-button markup is still present alongside the select")
	}
}

// The flag that hides the submit button is set in the head, not in app.js.
// app.js loads at the end of <body>, so the button would render, be seen, and
// then vanish — a visible flicker on every page load.
func TestJSFlagIsSetInTheHeadNotInAppJS(t *testing.T) {
	root := filepath.Dir(localesDir(t))
	base, err := os.ReadFile(filepath.Join(root, "web", "templates", "base.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(base), "data-js") {
		t.Error("base.html never sets data-js; the no-JS submit button would always show")
	}

	appJS, err := os.ReadFile(filepath.Join(root, "web", "static", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(appJS), `setAttribute('data-js'`) {
		t.Error("app.js sets data-js; it loads at the end of body, so the button would flicker")
	}
}
