package web

import (
	"os"
	"path/filepath"
	"regexp"
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

// templatesDir is web/templates, resolved off the same repository root
// localesDir already knows how to find.
func templatesDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(filepath.Dir(localesDir(t)), "web", "templates")
}

// allTemplateSources reads every *.html file in web/templates, keyed by name.
// Fix round 1 read only base.html here and so never noticed that
// login.html, login_verify.html and firstrun.html each carry their own <head>
// with their own, separately unsynchronised copy of the theme script.
func allTemplateSources(t *testing.T) map[string]string {
	t.Helper()
	dir := templatesDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	sources := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		sources[e.Name()] = string(raw)
	}
	if len(sources) == 0 {
		t.Fatal("web/templates has no .html files")
	}
	return sources
}

var templateRefRE = regexp.MustCompile(`{{template "([\w.-]+)" \.}}`)

// definedTemplateBody returns the body of {{define "name"}}...{{end}} wherever
// it appears among sources, or "" if name is never defined anywhere. Bodies
// looked up here — themeScript today — are flat, with no {{if}}/{{range}} of
// their own, so the first {{end}} is the matching one; langswitchBlock above
// needs a different rule for exactly that reason, because its body nests one.
func definedTemplateBody(sources map[string]string, name string) string {
	marker := `{{define "` + name + `"}}`
	for _, src := range sources {
		start := strings.Index(src, marker)
		if start < 0 {
			continue
		}
		rest := src[start+len(marker):]
		if end := strings.Index(rest, "{{end}}"); end >= 0 {
			return rest[:end]
		}
	}
	return ""
}

// setsDataJS reports whether src sets data-js itself, or delegates to exactly
// one other {{template "name" .}} whose body does. One hop is what the shared
// theme script needs today (base.html, login.html, login_verify.html and
// firstrun.html each call {{template "themeScript" .}} directly); it does not
// recurse further, so a second layer of indirection added later would need
// this helper extended rather than silently passing.
func setsDataJS(sources map[string]string, src string) bool {
	if strings.Contains(src, "data-js") {
		return true
	}
	for _, m := range templateRefRE.FindAllStringSubmatch(src, -1) {
		if body := definedTemplateBody(sources, m[1]); body != "" && strings.Contains(body, "data-js") {
			return true
		}
	}
	return false
}

// TestLangSwitchHeadsSetDataJS enumerates every template that renders the
// language switch by scanning for the include, rather than naming the sites
// by hand — the previous version of this test (TestJSFlagIsSetInTheHeadNotInAppJS)
// read base.html alone, which is exactly why login.html, login_verify.html and
// firstrun.html each shipped their own copy of the theme script with no
// data-js line: a check that looks general but examines one instance.
func TestLangSwitchHeadsSetDataJS(t *testing.T) {
	sources := allTemplateSources(t)

	var includesLangswitch []string
	for name, src := range sources {
		if strings.Contains(src, `{{template "langswitch" .}}`) {
			includesLangswitch = append(includesLangswitch, name)
		}
	}
	if len(includesLangswitch) == 0 {
		t.Fatal(`no template includes {{template "langswitch" .}} — did the include site move?`)
	}

	for _, name := range includesLangswitch {
		if !setsDataJS(sources, sources[name]) {
			t.Errorf("%s includes the language switch but never sets data-js (directly or via a "+
				"{{template}} it calls) — its .lang-submit stays visible even once a script is running", name)
		}
	}
}

// The flag that hides the submit button must never be set from app.js: it
// loads at the end of <body>, so a flag set there would let the button
// render, be seen, and then vanish — a visible flicker on every page load.
func TestAppJSNeverSetsDataJS(t *testing.T) {
	root := filepath.Dir(localesDir(t))
	appJS, err := os.ReadFile(filepath.Join(root, "web", "static", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(appJS), `setAttribute('data-js'`) {
		t.Error("app.js sets data-js; it loads at the end of body, so the button would flicker")
	}
}
