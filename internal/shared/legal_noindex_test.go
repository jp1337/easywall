package shared

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The legal pages render the controller's name and postal address
// (controller.html, filled from the LEGAL_YML secret at deploy). Those pages
// must never be offered to a search index: a sitemap entry invites Google to
// crawl them and a missing noindex lets it keep the result, so the controller's
// personal details end up published far beyond the one page the law requires.
//
// The exclusion is two source-level facts, both regressible by deleting one
// front-matter line:
//
//   - noindex: true   -> default.html emits <meta name="robots" content="noindex">
//   - sitemap: false  -> jekyll-sitemap omits the page from sitemap.xml
//
// This derives its own list — every page that includes controller.html — so a
// new legal page that embeds the controller block is caught without editing the
// test. robots.txt is deliberately left at Allow: / and is not asserted here:
// a Disallow would stop the crawler reading the noindex it must obey.
func TestPagesWithTheControllerBlockAreExcludedFromSearch(t *testing.T) {
	docs := filepath.Join("..", "..", "docs")

	// default.html is the mechanism for the noindex meta; without this the
	// front-matter flag renders nothing.
	layout, err := os.ReadFile(filepath.Join(docs, "_layouts", "default.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(layout), `page.noindex`) ||
		!strings.Contains(string(layout), `name="robots" content="noindex"`) {
		t.Error("default.html does not emit a noindex robots meta keyed on page.noindex")
	}

	var withController []string
	err = filepath.WalkDir(docs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// _site is a build artefact; vendor is the bundled gems.
			if name := d.Name(); name == "_site" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, ".html") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "controller.html") {
			withController = append(withController, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(withController) == 0 {
		t.Fatal("no page includes controller.html — the test's list derivation is broken")
	}

	for _, path := range withController {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		fm := frontMatter(string(data))
		if !strings.Contains(fm, "noindex: true") {
			t.Errorf("%s renders the controller block but its front matter lacks `noindex: true`", path)
		}
		if !strings.Contains(fm, "sitemap: false") {
			t.Errorf("%s renders the controller block but its front matter lacks `sitemap: false`", path)
		}
	}
}

// frontMatter returns the YAML between the first two `---` lines, or "".
func frontMatter(doc string) string {
	if !strings.HasPrefix(doc, "---\n") {
		return ""
	}
	rest := doc[len("---\n"):]
	if end := strings.Index(rest, "\n---"); end >= 0 {
		return rest[:end]
	}
	return ""
}
