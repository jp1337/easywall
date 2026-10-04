package shared

import (
	"regexp"
	"strings"
	"testing"
)

// The AI disclosure: what AGENTS.md and the templates ask agents to write is what
// ai-disclosure.yml labels. On 2026-10-04 a sister project received an issue
// asking to be paid for a finding and a pull request with fabricated test
// claims, both from a bot, neither saying so. The label is a review signal, not
// a block — a bot that never reads the repository is not caught.
var askingForTheDisclosure = []string{
	"AGENTS.md",
	"CONTRIBUTING.md",
	".github/pull_request_template.md",
	".github/ISSUE_TEMPLATE/bug_report.md",
	".github/ISSUE_TEMPLATE/feature_request.md",
}

func disclosurePattern(t *testing.T) *regexp.Regexp {
	t.Helper()
	wf := repoFile(t, ".github", "workflows", "ai-disclosure.yml")
	found := regexp.MustCompile(`grep -Pzq '([^']+)'`).FindStringSubmatch(wf)
	if found == nil {
		t.Fatal("ai-disclosure.yml no longer matches with grep -Pzq '<pattern>'")
	}
	// grep -P's \t and \n mean the same in Go's RE2.
	return regexp.MustCompile(found[1])
}

// disclosureIn returns the two-line disclosure as the file shows it, indentation
// stripped — the shape an agent copies into a description.
func disclosureIn(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "> [!WARNING]" && i+1 < len(lines) {
			return "> [!WARNING]\n" + strings.TrimSpace(lines[i+1])
		}
	}
	return ""
}

func TestEveryPlaceAsksForWhatTheWorkflowLabels(t *testing.T) {
	pattern := disclosurePattern(t)
	for _, path := range askingForTheDisclosure {
		if !pattern.MatchString(disclosureIn(repoFile(t, path))) {
			t.Errorf("%s asks for a disclosure ai-disclosure.yml does not recognise", path)
		}
	}
}

func TestTheTemplatesHideTheRequestInAComment(t *testing.T) {
	comment := regexp.MustCompile(`(?s)<!--(.*?)-->`)
	for _, path := range askingForTheDisclosure[2:] {
		text := repoFile(t, path)
		m := comment.FindStringSubmatch(text)
		if m == nil || !strings.Contains(m[1], "AI-generated") {
			t.Errorf("%s: the request is not inside an HTML comment", path)
			continue
		}
		if strings.Contains(strings.Replace(text, m[0], "", 1), "AI-generated") {
			t.Errorf("%s: AI-generated is visible outside the comment, so every human-written description would carry it", path)
		}
	}
}

func TestOrdinaryTextIsNotLabelled(t *testing.T) {
	pattern := disclosurePattern(t)
	for _, text := range []string{"Fixes #1.", "> [!WARNING]\n> Breaking change", "This is not AI-generated."} {
		if pattern.MatchString(text) {
			t.Errorf("labelled: %q", text)
		}
	}
}

// pull_request_target carries a write token; the fork's text may reach the shell
// only through env, and nothing of the fork may be checked out.
func TestTheDisclosureWorkflowNeverRunsTheText(t *testing.T) {
	wf := repoFile(t, ".github", "workflows", "ai-disclosure.yml")
	if strings.Contains(wf, "actions/checkout") {
		t.Error("ai-disclosure.yml checks out code under pull_request_target")
	}
	_, run, ok := strings.Cut(wf, "run: |")
	if !ok {
		t.Fatal("ai-disclosure.yml has no run: | block")
	}
	if strings.Contains(run, "${{") {
		t.Error("ai-disclosure.yml interpolates into run:; event text must reach the shell only through env")
	}
}
