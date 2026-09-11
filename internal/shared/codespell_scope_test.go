package shared

import (
	"strings"
	"testing"
)

// TestTheSpellingGateRunsWhereItIsConfigured keeps the gate's scope and its
// configuration in agreement.
//
// .codespellrc is written repo-wide. For four releases the only job that ran
// codespell was docs.yml's, which triggers on docs/**, CHANGELOG.md,
// .github/actions/** and itself — so a typo in README.md, DESIGN.md,
// locales/en.json or any Go file was never checked, and nothing said so.
//
// It asserts two things, because either one alone permits the old state: the
// unfiltered workflow runs it, and the path-filtered one does not.
func TestTheSpellingGateRunsWhereItIsConfigured(t *testing.T) {
	// A "run:" line is required, not just the word anywhere in the file — the
	// surrounding comment explaining why codespell lives here also contains the
	// word, and would otherwise let this pass even with the step disabled.
	testWF := repoFile(t, ".github", "workflows", "test.yml")
	runsCodespell := false
	for _, line := range strings.Split(testWF, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, "run:") && strings.Contains(trimmed, "codespell") {
			runsCodespell = true
			break
		}
	}
	if !runsCodespell {
		t.Error("test.yml does not run codespell. It is the workflow with no path filter, " +
			"which is what makes a repo-wide dictionary actually cover the repository")
	}
	// docs.yml legitimately mentions ".codespellrc" in its trigger paths (a pull
	// request that only touches the config should still run the docs build), so
	// the search excludes that filename and looks for an actual invocation.
	docsWF := repoFile(t, ".github", "workflows", "docs.yml")
	for _, line := range strings.Split(docsWF, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(line, "codespell") && !strings.Contains(line, ".codespellrc") &&
			!strings.HasPrefix(trimmed, "#") {
			t.Errorf("docs.yml runs codespell again (%q) — it is path-filtered, so this copy "+
				"covers less than the configuration claims, and two copies disagree the "+
				"moment one is changed", trimmed)
		}
	}
}
