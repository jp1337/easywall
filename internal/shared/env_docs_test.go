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
func TestTheEnvironmentPageStatesTheRightTypes(t *testing.T) {
	page := repoFile(t, "docs", "_docs", "environment.md")
	for _, v := range WebEnvVars {
		if v.Kind != EnvBool {
			continue
		}
		// The row for a boolean variable has to say so somewhere on its line.
		var row string
		for _, line := range strings.Split(page, "\n") {
			if strings.Contains(line, v.Name) {
				row = line
				break
			}
		}
		if row == "" {
			continue // TestEveryEnvVarIsDocumented reports the missing row
		}
		if !strings.Contains(strings.ToLower(row), "bool") {
			t.Errorf("%s is EnvBool in the code, and its row does not say so:\n  %s",
				v.Name, row)
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
