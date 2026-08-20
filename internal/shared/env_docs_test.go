package shared

import (
	"regexp"
	"strings"
	"testing"
)

// The page exists so somebody can find the list. A variable missing from it is
// a variable nobody knows about; a variable on it that the code does not read
// is worse, because the reader will set it and believe it took effect.
//
// The same shape as TestEveryConfigKeyIsDocumented, applied to the environment.
func TestEveryEnvVarIsDocumented(t *testing.T) {
	page := repoFile(t, "docs", "_docs", "environment.md")

	var names []string
	for _, v := range CoreEnvVars {
		names = append(names, v.Name)
	}
	for _, v := range WebEnvVars {
		names = append(names, v.Name)
	}

	documented := map[string]bool{}
	for _, m := range regexp.MustCompile(`EASYWALL_[A-Z_]+`).FindAllString(page, -1) {
		documented[m] = true
	}

	for _, n := range names {
		if !documented[n] {
			t.Errorf("%s is read by the code and absent from docs/_docs/environment.md", n)
		}
		delete(documented, n)
	}
	for n := range documented {
		t.Errorf("docs/_docs/environment.md documents %s, which nothing reads", n)
	}
}

// Kind is not decoration: the page's type column has to agree with it.
//
// Without this the field is dead weight — every Set closure parses for itself,
// so nothing in the code reads Kind at all. Deriving the documented type from it
// is what stops the page saying "string" for a variable the code parses as a
// boolean, which is the mistake that sends somebody debugging a value that was
// rejected at startup for a reason the page denied.
//
// Both tables, and the core one only because it was checked and found to have no
// Type column at all: this loop ran over WebEnvVars alone, so core could have
// grown a boolean and the page would have stayed silent about it with every test
// green.
func TestTheEnvironmentPageStatesTheRightTypes(t *testing.T) {
	page := repoFile(t, "docs", "_docs", "environment.md")

	// The two tables carry different config types, so their rows are checked by
	// name and kind rather than by walking one slice.
	type documented struct {
		name string
		kind EnvKind
	}
	var vars []documented
	for _, v := range CoreEnvVars {
		vars = append(vars, documented{v.Name, v.Kind})
	}
	for _, v := range WebEnvVars {
		vars = append(vars, documented{v.Name, v.Kind})
	}

	want := map[EnvKind]string{EnvString: "string", EnvBool: "bool"}
	for _, v := range vars {
		stated, ok := want[v.kind]
		if !ok {
			t.Errorf("%s has kind %v, which this test has no wording for", v.name, v.kind)
			continue
		}
		// The row for a variable has to state its type somewhere on its line.
		var row string
		for _, line := range strings.Split(page, "\n") {
			if strings.Contains(line, v.name) {
				row = line
				break
			}
		}
		if row == "" {
			continue // TestEveryEnvVarIsDocumented reports the missing row
		}
		if !strings.Contains(strings.ToLower(row), stated) {
			t.Errorf("%s is %s in the code, and its row does not say so:\n  %s",
				v.name, stated, row)
		}
	}
}

// A page with no nav entry is reachable only by its URL, which for a page whose
// whole purpose is being findable is the same as not adding it.
func TestTheEnvironmentPageIsInTheNav(t *testing.T) {
	cfg := repoFile(t, "docs", "_config.yml")
	if !strings.Contains(cfg, "/docs/environment/") {
		t.Error("docs/_config.yml has no nav entry pointing at /docs/environment/")
	}
}

// TZ is not easywall's variable — the Go runtime reads it through tzdata. An
// operator who sets it needs to know it works; a maintainer grepping for it
// needs to know why it is not in the tables.
func TestTheEnvironmentPageExplainsTZ(t *testing.T) {
	page := repoFile(t, "docs", "_docs", "environment.md")
	if !strings.Contains(page, "TZ") {
		t.Error("docs/_docs/environment.md never mentions TZ")
	}
}
