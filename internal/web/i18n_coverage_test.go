package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// localesDir walks up from the package directory to the repository root, which
// is where locales/ lives regardless of where `go test` was invoked from.
func localesDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		candidate := filepath.Join(dir, "locales")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not locate locales/ above the package directory")
	return ""
}

// strictLangSet reports whether each StrictLangs member is present, for tests
// that hard-fail on the strict languages but only report gaps elsewhere.
func strictLangSet() map[string]bool {
	strict := make(map[string]bool, len(StrictLangs))
	for _, l := range StrictLangs {
		strict[l] = true
	}
	return strict
}

func localeIDs(t *testing.T, lang string) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(localesDir(t), lang+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct {
		ID          string `json:"id"`
		Translation string `json:"translation"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("locales/%s.json: %v", lang, err)
	}
	ids := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.Translation == "" {
			t.Errorf("locales/%s.json: %q has an empty translation", lang, e.ID)
		}
		if ids[e.ID] {
			t.Errorf("locales/%s.json: %q appears twice", lang, e.ID)
		}
		ids[e.ID] = true
	}
	return ids
}

var tCallRe = regexp.MustCompile(`\bT\s+"([a-z0-9_]+)"`)

// Every message id a template asks for must exist in the strict languages: T
// falls back to returning the id, so a typo or a key added to en.json only does
// not fail anywhere — it silently renders "ports_syntax_hint" to the operator.
// A language outside StrictLangs may have gaps — the request falls back to
// English rather than the raw id, since "en" is always the last candidate the
// localizer tries — so a missing key there is reported, not failed.
func TestTemplatesOnlyUseTranslatedKeys(t *testing.T) {
	dir := filepath.Join(filepath.Dir(localesDir(t)), "web", "templates")
	files, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no templates found in %s (err=%v)", dir, err)
	}

	codes, err := LocaleCodes(localesDir(t))
	if err != nil {
		t.Fatal(err)
	}
	langs := make(map[string]map[string]bool, len(codes))
	for _, code := range codes {
		langs[code] = localeIDs(t, code)
	}
	used := make(map[string][]string) // id -> templates referencing it

	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range tCallRe.FindAllStringSubmatch(string(body), -1) {
			used[m[1]] = append(used[m[1]], filepath.Base(f))
		}
	}
	if len(used) == 0 {
		t.Fatal("no T calls found — the regex or the templates changed shape")
	}

	strict := strictLangSet()
	for _, lang := range codes {
		var missing []string
		for id := range used {
			if !langs[lang][id] {
				missing = append(missing, id+" ("+used[id][0]+")")
			}
		}
		sort.Strings(missing)
		for _, m := range missing {
			if strict[lang] {
				t.Errorf("locales/%s.json is missing %s", lang, m)
			} else {
				t.Logf("locales/%s.json is missing %s", lang, m)
			}
		}
	}
}

// The strict languages must describe the same interface. A key in one and not
// the other means one language falls back to raw ids on some page.
func TestLocaleFilesAreAtParity(t *testing.T) {
	if len(StrictLangs) != 2 {
		t.Fatalf("TestLocaleFilesAreAtParity compares exactly two languages, StrictLangs has %v", StrictLangs)
	}
	a, b := StrictLangs[0], StrictLangs[1]
	idsA, idsB := localeIDs(t, a), localeIDs(t, b)

	var onlyA, onlyB []string
	for id := range idsA {
		if !idsB[id] {
			onlyA = append(onlyA, id)
		}
	}
	for id := range idsB {
		if !idsA[id] {
			onlyB = append(onlyB, id)
		}
	}
	sort.Strings(onlyA)
	sort.Strings(onlyB)
	if len(onlyA) > 0 {
		t.Errorf("in %s.json but not %s.json: %v", a, b, onlyA)
	}
	if len(onlyB) > 0 {
		t.Errorf("in %s.json but not %s.json: %v", b, a, onlyB)
	}
}

// A translation that is byte-identical to English is usually a forgotten one.
// Some genuinely are the same word — "Demo", "System", protocol names — so
// this is an allow-list, not a ban: adding to it should be a deliberate act.
//
// A language whose status is not reviewed is exempt: an in-house draft
// legitimately contains untranslated fragments, and status.json is where that
// is recorded. This test is what catches pasted English in a language that
// claims to be finished.
func TestTranslationsAreNotCopiedEnglish(t *testing.T) {
	same := map[string]bool{
		"demo_label": true, "nav_group_system": true, "nav_system": true,
		"ports_tcp": true, "ports_udp": true, "log_col_user": true,
		"forwarding_protocol": true, "dashboard_status": true,
		"system_title": true, "audit_title": true,
	}

	read := func(lang string) map[string]string {
		raw, err := os.ReadFile(filepath.Join(localesDir(t), lang+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var entries []struct {
			ID          string `json:"id"`
			Translation string `json:"translation"`
		}
		if err := json.Unmarshal(raw, &entries); err != nil {
			t.Fatal(err)
		}
		out := make(map[string]string, len(entries))
		for _, e := range entries {
			out[e.ID] = e.Translation
		}
		return out
	}

	dir := localesDir(t)
	codes, err := LocaleCodes(dir)
	if err != nil {
		t.Fatal(err)
	}
	status, err := LoadLocaleStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	en := read("en")

	for _, lang := range codes {
		if lang == "en" {
			continue
		}
		if !status[lang].Reviewed {
			t.Logf("%s is not reviewed — skipping the copied-English check", lang)
			continue
		}
		other := read(lang)
		var suspects []string
		for id, text := range en {
			if len(text) < 12 || same[id] {
				continue
			}
			if other[id] == text {
				suspects = append(suspects, id)
			}
		}
		sort.Strings(suspects)
		for _, id := range suspects {
			t.Errorf("%q is identical in en and %s — untranslated, or add it to the allow-list", id, lang)
		}
	}
}

// styleClasses returns every class selector defined in the built stylesheet.
func styleClasses(t *testing.T) map[string]bool {
	t.Helper()
	root := filepath.Dir(localesDir(t))
	raw, err := os.ReadFile(filepath.Join(root, "web", "static", "style.css"))
	if err != nil {
		t.Fatalf("read built stylesheet: %v", err)
	}
	// Tailwind escapes a dot inside a class name (.mt-1\.5), so the escape has to
	// be matched and then removed: a bare dot separates compound selectors, an
	// escaped one is part of the name.
	out := make(map[string]bool)
	for _, m := range regexp.MustCompile(`\.((?:[A-Za-z0-9_-]|\\.)+)`).
		FindAllStringSubmatch(string(raw), -1) {
		out[strings.ReplaceAll(m[1], `\`, "")] = true
	}
	return out
}

// assertAlertVariant checks that a response uses an alert variant the stylesheet
// actually defines.
//
// This exists because it did not. The two live-validation endpoints emitted
// alert-success, alert-error, alert-info and alert-soft — daisyUI names that
// stopped existing when the design system replaced daisyUI. The tests asserted
// those same dead names, so they passed while every validation response an
// operator saw rendered as an unstyled box.
func assertAlertVariant(t *testing.T, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Errorf("expected %s in the response, got: %s", want, body)
	}
	if !styleClasses(t)[want] {
		t.Errorf("%s is not defined in web/static/style.css", want)
	}
}

// Every class any template names must exist in the built stylesheet. A template
// referring to a class that was renamed or removed fails silently: the element
// renders, just unstyled.
func TestTemplateClassesExistInStylesheet(t *testing.T) {
	root := filepath.Dir(localesDir(t))
	files, err := filepath.Glob(filepath.Join(root, "web", "templates", "*.html"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no templates found (err=%v)", err)
	}
	defined := styleClasses(t)

	// Utilities Tailwind only emits when it sees them, plus hooks that exist for
	// JavaScript and carry no styling of their own.
	ignore := map[string]bool{
		"f-port": true, "f-ssh": true, "f-desc": true, "f-proto": true,
		"f-src": true, "f-dst": true, "del-rule": true, "inline": true,
	}

	classRe := regexp.MustCompile(`class="([^"{}]*)"`)
	missing := map[string][]string{}
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range classRe.FindAllStringSubmatch(string(body), -1) {
			for _, cls := range strings.Fields(m[1]) {
				if cls == "" || ignore[cls] || defined[cls] {
					continue
				}
				missing[cls] = append(missing[cls], filepath.Base(f))
			}
		}
	}
	names := make([]string, 0, len(missing))
	for cls := range missing {
		names = append(names, cls)
	}
	sort.Strings(names)
	for _, cls := range names {
		t.Errorf("class %q (%s) is not in web/static/style.css — renamed, removed, or a typo",
			cls, missing[cls][0])
	}
}

// A locale string written with markup markers has to be rendered by something
// that understands them.
//
// `code` and *emphasis* only become markup when the template puts the string
// through richText. With a plain {{T "id"}} the markers reach the page as
// literal backticks and asterisks — which is what two of the IPv6 settings
// strings did, and no test noticed because both the locale file and the
// template were, in isolation, perfectly fine.
func TestMarkupStringsAreRenderedThroughRichText(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(localesDir(t), "en.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct {
		ID          string `json:"id"`
		Translation string `json:"translation"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}

	markupRe := regexp.MustCompile("`[^`]+`|\\*[^*]+\\*")

	dir := filepath.Join(filepath.Dir(localesDir(t)), "web", "templates")
	files, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no templates found in %s (err=%v)", dir, err)
	}
	templates := make(map[string]string, len(files))
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		templates[filepath.Base(f)] = string(body)
	}

	for _, e := range entries {
		if !markupRe.MatchString(e.Translation) {
			continue
		}
		plain := regexp.MustCompile(`\{\{\s*T\s+"` + regexp.QuoteMeta(e.ID) + `"\s*\}\}`)
		for name, src := range templates {
			if plain.MatchString(src) {
				t.Errorf("%s: %q contains markup but is rendered with plain T; "+
					"use richText (T %q) or drop the markers", name, e.ID, e.ID)
			}
		}
	}
}

// app.js builds text in the browser, so the strings it needs are inlined into
// every page from clientStringKeys. Two ways for that to drift, both silent:
// a key app.js asks for that is not in the list, and a key in the list that no
// locale file has. str() falls back to the key itself, so either one renders
// "state_unknown" to the operator instead of a sentence.
func TestClientStringsCoverWhatAppJSAsksFor(t *testing.T) {
	root := filepath.Dir(localesDir(t))
	appJS, err := os.ReadFile(filepath.Join(root, "web", "static", "app.js"))
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}

	shipped := make(map[string]bool, len(clientStringKeys))
	for _, k := range clientStringKeys {
		shipped[k] = true
	}

	strCallRe := regexp.MustCompile(`\bstr\('([a-z0-9_]+)'\)`)
	asked := make(map[string]bool)
	for _, m := range strCallRe.FindAllStringSubmatch(string(appJS), -1) {
		asked[m[1]] = true
	}
	if len(asked) == 0 {
		t.Fatal("no str() calls found — the regex or app.js changed shape")
	}

	for key := range asked {
		if !shipped[key] {
			t.Errorf("app.js asks for %q, which clientStringKeys does not ship", key)
		}
	}

	// The other direction is worth knowing too: a key shipped to every page and
	// used by nothing is dead weight in the blob.
	for key := range shipped {
		if !asked[key] {
			t.Errorf("clientStringKeys ships %q, which app.js never asks for", key)
		}
	}

	// Every shipped key must exist in the strict languages — that failure is
	// hard, because en/de must never fall back to a raw key. Anything else may
	// have gaps, so report what is missing without failing the build.
	strict := strictLangSet()
	codes, err := LocaleCodes(localesDir(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range codes {
		ids := localeIDs(t, lang)
		for key := range shipped {
			if ids[key] {
				continue
			}
			if strict[lang] {
				t.Errorf("locales/%s.json has no %q, so app.js would render the key itself", lang, key)
			} else {
				t.Logf("locales/%s.json has no %q, so app.js would render the key itself", lang, key)
			}
		}
	}
}
