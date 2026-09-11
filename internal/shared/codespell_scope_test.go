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
	// docs.yml legitimately mentions ".codespellrc" as a trigger-path list entry
	// (a pull request that only touches the config should still run the docs
	// build) — that exact line, trimmed, and nothing looser: an invocation that
	// happens to also name the file, e.g. `run: pipx run codespell .codespellrc`,
	// is exactly the anti-pattern this repository's commit history warns about
	// (an explicit path bypasses the skip list's ./-prefixed entries) and must
	// still be caught. So an invocation is any non-comment line containing
	// "run:" and "codespell" together, full stop — mirroring the test.yml half.
	docsWF := repoFile(t, ".github", "workflows", "docs.yml")
	for _, line := range strings.Split(docsWF, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == `- ".codespellrc"` {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, "run:") && strings.Contains(trimmed, "codespell") {
			t.Errorf("docs.yml runs codespell again (%q) — it is path-filtered, so this copy "+
				"covers less than the configuration claims, and two copies disagree the "+
				"moment one is changed", trimmed)
		}
	}
}
