package shared

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The deploy job in docs.yml is the last thing between a build and the public
// site. Two of its steps are the only guard that the German legal pages
// (Impressum/Datenschutz/Privacy) actually carry the controller block § 5 DDG
// requires before they go live. Both were silently vacuous:
//
//   - "The legal pages carry the controller" grepped each page for the
//     placeholder text. grep -q on an absent file exits 2, which `if` reads as
//     "no match", so a page that stopped building passed the gate and deployed
//     missing — the exact empty-site shape the sibling "The site is not empty"
//     step exists to stop, but that step only counts pages and checks index.html.
//   - "Legal contact from the deployment secret" wrote legal.yml from the secret
//     and checked only that the secret was non-empty. controller.html renders the
//     placeholder unless legal.lines is truthy, so a present-but-malformed secret
//     (no lines: key) shipped the placeholder with CI green.
//
// These run the real scripts out of the workflow against crafted inputs. Testing
// the shipped text, not a copy, is the point: the gate is one dedent away from
// vacuous again.

// workflowStepScript returns the shell body of the `run: |` block of the named
// step in the given workflow file.
func workflowStepScript(t *testing.T, workflow, stepName string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", workflow))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")

	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "- name: "+stepName {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("step %q not found in %s", stepName, workflow)
	}

	runIdx, runIndent := -1, 0
	for i := start + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "- name:" || strings.HasPrefix(strings.TrimSpace(lines[i]), "- name:") {
			break // next step, no run block in this one
		}
		if strings.TrimSpace(lines[i]) == "run: |" {
			runIdx = i
			runIndent = len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
			break
		}
	}
	if runIdx < 0 {
		t.Fatalf("step %q has no `run: |` block", stepName)
	}

	var body []string
	for i := runIdx + 1; i < len(lines); i++ {
		l := lines[i]
		if strings.TrimSpace(l) == "" {
			body = append(body, "")
			continue
		}
		indent := len(l) - len(strings.TrimLeft(l, " "))
		if indent <= runIndent {
			break
		}
		body = append(body, l)
	}

	// Dedent by the least-indented non-blank line.
	min := 1 << 30
	for _, l := range body {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if n := len(l) - len(strings.TrimLeft(l, " ")); n < min {
			min = n
		}
	}
	for i, l := range body {
		if len(l) >= min {
			body[i] = l[min:]
		}
	}
	return strings.Join(body, "\n")
}

func runScript(t *testing.T, dir, script string, env ...string) int {
	t.Helper()
	cmd := exec.Command("bash", "-c", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	err := cmd.Run()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	t.Fatalf("running script: %v", err)
	return -1
}

func TestLegalGate_FailsWhenAPageIsMissing(t *testing.T) {
	script := workflowStepScript(t, "docs.yml", "The legal pages carry the controller")

	filled := "<html><body><p>Max Mustermann<br>Musterstr. 1, Germany</p></body></html>"
	placeholder := "<html><body><p><em>Name and postal address are filled in when the site is deployed.</em></p></body></html>"

	write := func(dir, page, html string) {
		p := filepath.Join(dir, "_site", page)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "index.html"), []byte(html), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("all pages present and filled passes", func(t *testing.T) {
		dir := t.TempDir()
		for _, p := range []string{"legal-notice", "privacy", "datenschutz"} {
			write(dir, p, filled)
		}
		if rc := runScript(t, dir, script); rc != 0 {
			t.Fatalf("filled pages must pass, got exit %d", rc)
		}
	})

	t.Run("a missing page fails", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "legal-notice", filled)
		write(dir, "privacy", filled)
		// datenschutz never built — the deploy would ship without it.
		if rc := runScript(t, dir, script); rc == 0 {
			t.Fatal("a legal page missing from _site must fail the gate, but it passed")
		}
	})

	t.Run("a placeholder page fails", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "legal-notice", filled)
		write(dir, "privacy", filled)
		write(dir, "datenschutz", placeholder)
		if rc := runScript(t, dir, script); rc == 0 {
			t.Fatal("a page still showing the placeholder must fail the gate, but it passed")
		}
	})
}

func TestLegalSecret_FailsWithoutALinesKey(t *testing.T) {
	script := workflowStepScript(t, "docs.yml", "Legal contact from the deployment secret")

	t.Run("a secret with lines passes", func(t *testing.T) {
		dir := t.TempDir()
		if rc := runScript(t, dir, script, "LEGAL_YML=lines:\n  - \"Max Mustermann\"\n  - \"Musterstr. 1\""); rc != 0 {
			t.Fatalf("a well-formed secret must pass, got exit %d", rc)
		}
	})

	t.Run("an empty secret fails", func(t *testing.T) {
		dir := t.TempDir()
		if rc := runScript(t, dir, script, "LEGAL_YML="); rc == 0 {
			t.Fatal("an empty secret must fail")
		}
	})

	t.Run("a secret without a lines key fails", func(t *testing.T) {
		dir := t.TempDir()
		// Present and non-empty, but the template needs legal.lines — this renders
		// the placeholder onto the live Impressum.
		if rc := runScript(t, dir, script, "LEGAL_YML=name: Max Mustermann"); rc == 0 {
			t.Fatal("a secret without a top-level lines: key must fail, but it passed")
		}
	})
}
