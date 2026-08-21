package web

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// endonyms maps a locale code to the name a document calls that language by.
// Both forms, because a document may use either: the endonym is what the switch
// shows a reader, the English exonym is what prose about the project tends to
// reach for.
//
// Adding a language means adding a line here. The test below is what makes
// forgetting it fail rather than pass quietly.
var endonyms = map[string][]string{
	"en": {"English"},
	"de": {"Deutsch", "German"},
	"fr": {"Français", "French"},
}

// languageEnumerations are the documents that tell a reader which languages the
// interface speaks. Curated rather than derived, and that is the interesting
// decision: a rule scanning every file for a language name cannot tell an
// enumeration from a mention. `docs/_docs/roadmap.md` names French because a
// release is about French; CONTRIBUTING.md names German because German is the
// example of a link landing in a different place in the sentence. Neither
// claims to be the list, and requiring them to name every language would be
// requiring prose to be wrong.
//
// What keeps this list from going stale is TestNoDocumentPresentsAPartialSet
// below, not this list being clever.
var languageEnumerations = []string{
	"README.md",
	"CONTRIBUTING.md",
	filepath.Join("docs", "index.md"),
	filepath.Join("docs", "_docs", "index.md"),
	filepath.Join("docs", "_docs", "contributing.md"),
	filepath.Join("docs", "_docs", "configuration.md"),
}

// currentTruthDocs are the documents that describe what easywall is now, as
// opposed to what it was. CHANGELOG.md and everything under docs-tech/plans and
// docs-tech/specs are deliberately absent: a changelog entry saying "i18n
// support (English and German)" was true when it was written, and editing it to
// mention French would be falsifying a record rather than fixing a claim.
func currentTruthDocs(t *testing.T) []string {
	t.Helper()
	root := filepath.Dir(localesDir(t))

	var out []string
	for _, name := range []string{"README.md", "CONTRIBUTING.md", "DESIGN.md", "CLAUDE.md"} {
		out = append(out, filepath.Join(root, name))
	}
	docs := filepath.Join(root, "docs")
	err := filepath.WalkDir(docs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// docs/vendor is the Jekyll bundle: thousands of files, none of them ours.
		if d.IsDir() && d.Name() == "vendor" {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".md") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs/: %v", err)
	}
	if len(out) < 10 {
		t.Fatalf("found only %d current-truth documents — the tree moved", len(out))
	}
	return out
}

// offeredLanguages is what the interface actually speaks, read from the locale
// files rather than restated here.
func offeredLanguages(t *testing.T) []string {
	t.Helper()
	codes, err := LocaleCodes(localesDir(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range codes {
		if _, ok := endonyms[code]; !ok {
			t.Fatalf("locales/%s.json exists but endonyms has no name for it — add one, "+
				"or the documents cannot be checked against it", code)
		}
	}
	return codes
}

// namesLanguage reports whether body calls the language by any of its names.
func namesLanguage(body, code string) bool {
	for _, name := range endonyms[code] {
		if strings.Contains(body, name) {
			return true
		}
	}
	return false
}

// A page that lists the languages must list all of them. A feature table
// promising "English and German" while the switcher offers three is the same
// class of defect as a stale screenshot: the reader has no way to tell, and the
// one who does have a way to tell is the one who wrote it.
func TestEveryEnumerationNamesEveryLanguage(t *testing.T) {
	root := filepath.Dir(localesDir(t))
	offered := offeredLanguages(t)

	for _, name := range languageEnumerations {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Errorf("%s is listed as enumerating the languages but cannot be read: %v", name, err)
			continue
		}
		body := string(raw)
		for _, code := range offered {
			if !namesLanguage(body, code) {
				t.Errorf("%s enumerates the languages but never names %s (%s), which the interface offers",
					name, endonyms[code][0], code)
			}
		}
	}
}

// languageNameRe matches any name any language is called by.
var languageNameRe = regexp.MustCompile(`English|Deutsch|German|Français|French`)

// joinWordRe finds the words between two language names. Anything longer than
// two letters that is not one of the connectives below means the two names are
// in different clauses rather than in one list.
var joinWordRe = regexp.MustCompile(`[A-Za-zÀ-ÿ]{2,}`)

// joinWords are what may legitimately sit between two names inside one list —
// the conjunctions, the articles, and the locale codes, because
// `"en" (English) or "de" (German)` is a list with the codes spelled out.
var joinWords = map[string]bool{
	"and": true, "or": true, "und": true, "et": true, "oder": true,
	"the": true, "en": true, "de": true, "fr": true,
}

// Any document describing easywall today that still presents the strict pair as
// though it were the whole set has to be found, wherever it is. This is the
// guard on the curated list above: a new page that says "English and German"
// either gets fixed or gets added to `languageEnumerations` — either way
// somebody looks at it, which is the whole point.
//
// Six files said it when French was added, and the implementation plan for that
// release named four of them. docs/index.md — the landing page — was not one of
// the four.
//
// Scoped to StrictLangs on purpose, and this is the limit worth knowing: the
// claim being guarded is the one that existed because `en` and `de` were the
// only two, so a run naming only those is the defect. A run naming Deutsch and
// Français is not — that is how both the endonym rule and the config page give
// two examples, and demanding every language in a sentence about naming would
// be demanding that prose be wrong. Completeness of the pages that really do
// enumerate is the other test's job, not this one's.
func TestNoDocumentStillClaimsOnlyTheStrictPair(t *testing.T) {
	offered := offeredLanguages(t)
	if len(offered) <= len(StrictLangs) {
		t.Skip("nothing beyond the strict pair ships, so naming the pair is not yet a stale claim")
	}

	strict := strictLangSet()
	for _, path := range currentTruthDocs(t) {
		raw, err := os.ReadFile(path) // #nosec G304 -- a path from walking the repository
		if err != nil {
			t.Fatal(err)
		}

		for _, cluster := range languageClusters(string(raw)) {
			if len(cluster.codes) < 2 {
				continue // a single mention is not a list
			}
			onlyStrict := true
			for code := range cluster.codes {
				if !strict[code] {
					onlyStrict = false
					break
				}
			}
			if !onlyStrict {
				continue
			}
			var missing []string
			for _, code := range offered {
				if !cluster.codes[code] {
					missing = append(missing, endonyms[code][0])
				}
			}
			sort.Strings(missing)
			rel, _ := filepath.Rel(filepath.Dir(localesDir(t)), path)
			t.Errorf("%s presents %q as the set of languages, leaving out %s",
				rel, strings.Join(strings.Fields(cluster.text), " "), strings.Join(missing, ", "))
		}
	}
}

// cluster is a run of language names joined by nothing but conjunctions.
type cluster struct {
	text  string
	codes map[string]bool
}

// languageClusters finds each run of language names that reads as one list.
//
// Two names belong to the same run when the text between them carries no word
// of its own — "English and German", "English, Deutsch and Français",
// `"en" (English) or "de" (German)`. "German puts … where English has it third"
// is two clauses, and the words between them are what says so.
func languageClusters(body string) []cluster {
	hits := languageNameRe.FindAllStringIndex(body, -1)
	if len(hits) == 0 {
		return nil
	}

	code := func(name string) string {
		for c, names := range endonyms {
			for _, n := range names {
				if n == name {
					return c
				}
			}
		}
		return ""
	}

	var out []cluster
	start, current := 0, 0
	for i := range hits {
		if i > 0 {
			between := body[hits[i-1][1]:hits[i][0]]
			joined := true
			for _, w := range joinWordRe.FindAllString(between, -1) {
				if !joinWords[strings.ToLower(w)] {
					joined = false
					break
				}
			}
			if !joined {
				out = append(out, buildCluster(body, hits, start, current, code))
				start = i
			}
		}
		current = i
	}
	return append(out, buildCluster(body, hits, start, current, code))
}

func buildCluster(body string, hits [][]int, first, last int, code func(string) string) cluster {
	codes := make(map[string]bool)
	for i := first; i <= last; i++ {
		codes[code(body[hits[i][0]:hits[i][1]])] = true
	}
	return cluster{text: body[hits[first][0]:hits[last][1]], codes: codes}
}
