package shared

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// navPaths returns every path: entry in the nav: block of docs/_config.yml.
//
// The block is read by its own bounds rather than by parsing the whole file:
// Jekyll's config is YAML, the repository has no YAML dependency, and a nav
// entry is one line of the shape `path: /docs/features/ports/`. The bounds
// matter because `path:` is not a word only nav uses — reading the whole file
// would pick up anything else that ever names a path.
func navPaths(t *testing.T, cfg string) []string {
	t.Helper()

	lines := strings.Split(cfg, "\n")
	start := -1
	for i, line := range lines {
		if line == "nav:" {
			start = i + 1
			break
		}
	}
	if start < 0 {
		t.Fatal("docs/_config.yml has no `nav:` block; the sidebar is built from it")
	}

	// A top-level key ends the block. Everything inside nav is indented.
	topLevel := regexp.MustCompile(`^[A-Za-z_]`)
	var paths []string
	pathLine := regexp.MustCompile(`^\s+path:\s*(\S+)`)
	for _, line := range lines[start:] {
		if topLevel.MatchString(line) {
			break
		}
		if m := pathLine.FindStringSubmatch(line); m != nil {
			paths = append(paths, strings.Trim(m[1], `"'`))
		}
	}
	return paths
}

// docsPageURLs returns the URL every page in the docs collection is published
// at, derived the way Jekyll derives it: an explicit permalink if the page
// declares one, otherwise /docs/:path/ from the collection's permalink setting.
func docsPageURLs(t *testing.T, root string) map[string]string {
	t.Helper()

	collection := filepath.Join(root, "docs", "_docs")
	permalink := regexp.MustCompile(`(?m)^permalink:\s*(\S+)`)

	urls := map[string]string{}
	err := filepath.WalkDir(collection, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(collection, path)
		if err != nil {
			return err
		}
		url := "/docs/" + strings.TrimSuffix(filepath.ToSlash(rel), ".md") + "/"
		if m := permalink.FindStringSubmatch(string(raw)); m != nil {
			url = strings.Trim(m[1], `"'`)
		}
		urls[url] = filepath.ToSlash(filepath.Join("_docs", rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs/_docs: %v", err)
	}
	return urls
}

// The sidebar is the only way to discover a documentation page that the reader
// does not already know the name of. Search finds a page by its words; nothing
// finds a page that is in neither.
//
// TestTheEnvironmentPageIsInTheNav guards exactly one page this way, because
// that page was the one that had been forgotten. This is the same check applied
// to the collection instead of to a name — which is what makes it survive the
// next reorganisation of the sidebar: grouping the twenty-seven flat entries
// into sections moves every single path in the file, and a page dropped in that
// move looks like nothing at all in the diff.
//
// The reverse direction is checked too: a nav entry pointing at a page that does
// not exist is a 404 in the sidebar of every page on the site.
func TestEveryDocsPageIsInTheNav(t *testing.T) {
	root := repoRootDir(t)
	cfg := repoFile(t, "docs", "_config.yml")

	paths := navPaths(t, cfg)
	if len(paths) < 20 {
		t.Fatalf("found %d nav paths in docs/_config.yml; the sidebar has more than that, "+
			"so the pattern no longer matches how entries are written and this test "+
			"would pass by finding nothing", len(paths))
	}

	inNav := map[string]bool{}
	for _, p := range paths {
		inNav[p] = true
	}

	pages := docsPageURLs(t, root)
	if len(pages) < 20 {
		t.Fatalf("found %d pages under docs/_docs; the collection is larger than that", len(pages))
	}

	var missing []string
	for url, file := range pages {
		if !inNav[url] {
			missing = append(missing, url+"  (docs/"+file+")")
		}
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("no nav entry points at %s\n"+
			"  the page is published but reachable only by its URL — add it to nav: "+
			"in docs/_config.yml", m)
	}

	// Home is /, which is docs/index.md and not part of the collection. Every
	// other entry has to resolve to a page that exists.
	for _, p := range paths {
		if !strings.HasPrefix(p, "/docs/") {
			continue
		}
		if _, ok := pages[p]; !ok {
			t.Errorf("nav entry %s has no page in docs/_docs — the sidebar links to a 404 "+
				"on every page of the site", p)
		}
	}
}
