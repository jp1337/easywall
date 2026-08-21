package web

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// One language, stated once. The site said two: `en` on the 27 real pages and
// `en-US` on the 23 old-path redirect stubs, because jekyll-redirect-from
// renders those from its own template and that template hardcodes `en-US`.
//
// It surfaced as a search defect — Pagefind read two languages and built two
// indexes that could not see each other — but it is an accessibility defect on
// its own: a screen reader is told the wrong language on 23 of the site's URLs.
//
// Checked against the layouts rather than the built site, because the Go suite
// does not build Jekyll. A layout named `redirect` has to exist at all, or the
// plugin's built-in one is used again and nothing here would notice.
func TestEveryDocsLayoutDeclaresTheSiteLanguage(t *testing.T) {
	root := filepath.Dir(localesDir(t))
	dir := filepath.Join(root, "docs", "_layouts")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	langRe := regexp.MustCompile(`<html[^>]*\slang="([^"]*)"`)
	seen := map[string]string{} // lang -> first layout that used it
	sawRedirect := false

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".html" {
			continue
		}
		if e.Name() == "redirect.html" {
			sawRedirect = true
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name())) // #nosec G304 -- a layout this test enumerated
		if err != nil {
			t.Fatal(err)
		}
		m := langRe.FindSubmatch(raw)
		if m == nil {
			continue // a partial layout with no <html> of its own
		}
		lang := string(m[1])
		if _, ok := seen[lang]; !ok {
			seen[lang] = e.Name()
		}
	}

	if !sawRedirect {
		t.Error("docs/_layouts/redirect.html does not exist, so jekyll-redirect-from " +
			"renders the stubs from its own template, which hardcodes lang=\"en-US\"")
	}
	if len(seen) > 1 {
		t.Errorf("the layouts declare %d different languages: %v — Pagefind reads each "+
			"as its own index, and a search in one cannot see the other", len(seen), seen)
	}
	// Read from _config.yml rather than restated, because two sources for one
	// fact is what produced the split in the first place.
	cfg, err := os.ReadFile(filepath.Join(root, "docs", "_config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^lang:\s*(\S+)`).FindSubmatch(cfg)
	if m == nil {
		t.Fatal("docs/_config.yml declares no lang:, so there is nothing for the layouts to agree with")
	}
	want := string(m[1])
	if len(seen) == 1 && seen[want] == "" {
		t.Errorf("the layouts declare %v but docs/_config.yml says lang: %q", seen, want)
	}
}
