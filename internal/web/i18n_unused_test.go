package web

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A key nothing renders is a sentence nobody reads and every translator still
// translates — eight sat in all three catalogues (cancel, delete, no,
// dashboard_status, dashboard_update, blocklist_hint, allowlist_hint,
// custom_error_line), and one of them was on the copied-English allow-list.
//
// Used means: quoted whole in a template, in app.js or in non-test Go under
// internal/ or cmd/; or reached by a prefix a template builds with
// (printf "prefix_%s" …) or Go builds by concatenation ("prefix_" + …), or by a
// suffix Go appends (… + "_suffix").
func TestEveryLocaleKeyIsUsed(t *testing.T) {
	root := filepath.Dir(localesDir(t))
	var corpus strings.Builder
	add := func(path string) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		corpus.Write(raw)
		corpus.WriteByte('\n')
	}
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				add(path)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	templates, err := filepath.Glob(filepath.Join(root, "web", "templates", "*.html"))
	if err != nil || len(templates) == 0 {
		t.Fatalf("no templates found: %v", err)
	}
	for _, f := range templates {
		add(f)
	}
	add(filepath.Join(root, "web", "static", "app.js"))
	text := corpus.String()

	var prefixes, suffixes []string
	for _, m := range regexp.MustCompile(`printf "([a-z0-9_]+_)%s"|"([a-z0-9_]+_)" \+`).FindAllStringSubmatch(text, -1) {
		prefixes = append(prefixes, m[1]+m[2])
	}
	for _, m := range regexp.MustCompile(`\+ "(_[a-z0-9_]+)"`).FindAllStringSubmatch(text, -1) {
		suffixes = append(suffixes, m[1])
	}

	var unused []string
	for id := range localeStrings(t, "en") {
		if strings.Contains(text, `"`+id+`"`) || strings.Contains(text, "'"+id+"'") {
			continue
		}
		reached := false
		for _, p := range prefixes {
			reached = reached || strings.HasPrefix(id, p)
		}
		for _, s := range suffixes {
			reached = reached || (strings.HasSuffix(id, s) && strings.Contains(text, `"`+strings.TrimSuffix(id, s)+`"`))
		}
		if !reached {
			unused = append(unused, id)
		}
	}
	sort.Strings(unused)
	for _, id := range unused {
		t.Errorf("%q is in locales/en.json and nothing renders it — delete it from every catalogue", id)
	}
}
