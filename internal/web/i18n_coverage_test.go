package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

// Every message id a template asks for must exist in every locale file. T falls
// back to returning the id, so a typo or a key added to one file only does not
// fail anywhere — it silently renders "ports_syntax_hint" to the operator.
func TestTemplatesOnlyUseTranslatedKeys(t *testing.T) {
	dir := filepath.Join(filepath.Dir(localesDir(t)), "web", "templates")
	files, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no templates found in %s (err=%v)", dir, err)
	}

	langs := map[string]map[string]bool{"en": localeIDs(t, "en"), "de": localeIDs(t, "de")}
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

	for _, lang := range []string{"en", "de"} {
		var missing []string
		for id := range used {
			if !langs[lang][id] {
				missing = append(missing, id+" ("+used[id][0]+")")
			}
		}
		sort.Strings(missing)
		for _, m := range missing {
			t.Errorf("locales/%s.json is missing %s", lang, m)
		}
	}
}

// The two files must describe the same interface. A key in one and not the other
// means one language falls back to raw ids on some page.
func TestLocaleFilesAreAtParity(t *testing.T) {
	en, de := localeIDs(t, "en"), localeIDs(t, "de")

	var onlyEN, onlyDE []string
	for id := range en {
		if !de[id] {
			onlyEN = append(onlyEN, id)
		}
	}
	for id := range de {
		if !en[id] {
			onlyDE = append(onlyDE, id)
		}
	}
	sort.Strings(onlyEN)
	sort.Strings(onlyDE)
	if len(onlyEN) > 0 {
		t.Errorf("in en.json but not de.json: %v", onlyEN)
	}
	if len(onlyDE) > 0 {
		t.Errorf("in de.json but not en.json: %v", onlyDE)
	}
}

// A translation that is byte-identical in both languages is usually a forgotten
// one. Some genuinely are the same word — "Demo", "System", protocol names — so
// this is an allow-list, not a ban: adding to it should be a deliberate act.
func TestGermanTranslationsAreNotCopiedEnglish(t *testing.T) {
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

	en, de := read("en"), read("de")
	var suspects []string
	for id, text := range en {
		if len(text) < 12 || same[id] {
			continue
		}
		if de[id] == text {
			suspects = append(suspects, id)
		}
	}
	sort.Strings(suspects)
	for _, id := range suspects {
		t.Errorf("%q is identical in en and de — untranslated, or add it to the allow-list", id)
	}
}
