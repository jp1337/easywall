package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// repoScript returns a file from scripts/.
func repoScript(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Dir(localesDir(t))
	raw, err := os.ReadFile(filepath.Join(root, "scripts", name))
	if err != nil {
		t.Fatalf("read scripts/%s: %v", name, err)
	}
	return string(raw)
}

// The published screenshots are taken wide enough for the layout they document.
//
// `.page-grid` drops its 320px context column below a measured breakpoint, and
// the screenshot viewport was 1440 — 130px underneath it — from 2.11 to 2.13. So
// every figure in docs/ showed the collapsed single-column fallback: the aside
// cards stacked under the table on ports, blacklist, forwarding, custom and
// options alike, which is the layout a narrow window gets and not the one the
// design is about. Nothing failed; the screenshots were simply of the wrong
// thing, in the repository, for three releases.
//
// Both numbers are load-bearing and neither is near the other's file, which is
// exactly how they drifted: lowering the breakpoint is a stylesheet decision and
// changing the viewport is a script decision, and either one alone re-creates
// the defect.
func TestScreenshotsAreTakenAboveTheTwoColumnBreakpoint(t *testing.T) {
	css := appStylesheet(t)

	m := regexp.MustCompile(`@media \(max-width:(\d+)px\)\{\.page-grid\{grid-template-columns:minmax\(0,1fr\)\}`).
		FindStringSubmatch(css)
	if m == nil {
		t.Fatal("no `.page-grid` single-column media query in the built stylesheet; " +
			"this test can no longer tell what width the context column needs")
	}
	breakpoint, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("unreadable breakpoint %q: %v", m[1], err)
	}

	script := repoScript(t, "ui-check.mjs")
	sm := regexp.MustCompile(`SHOT_VIEWPORT = \{ width: (\d+), height: (\d+) \}`).FindStringSubmatch(script)
	if sm == nil {
		t.Fatal("scripts/ui-check.mjs no longer declares SHOT_VIEWPORT in the shape this " +
			"test reads; the screenshot viewport is unchecked")
	}
	width, err := strconv.Atoi(sm[1])
	if err != nil {
		t.Fatalf("unreadable screenshot width %q: %v", sm[1], err)
	}

	if width <= breakpoint {
		t.Errorf("screenshots are taken at %dpx, but .page-grid collapses to one column "+
			"at or below %dpx\n"+
			"  every figure in docs/ would show the narrow fallback layout, with the "+
			"aside cards stacked under the table instead of beside it", width, breakpoint)
	}
}

// A published screenshot is not a fullPage capture.
//
// `.sidebar` is `position: fixed` at `min-height: 100vh`. In a fullPage capture a
// fixed element stays laid out against the viewport it was rendered in, so on
// every page taller than the window the sidebar ended mid-image — the language
// switch, the theme toggle and Logout floating in the middle of a column that
// then went blank. Twenty-two of the thirty-four files in
// docs/assets/img/screens/ shipped that way.
//
// The fix is to grow the window to the document instead, so `shoot` must keep
// both halves: no fullPage, and a setViewportSize before the capture.
func TestScreenshotsGrowTheWindowInsteadOfCapturingBeyondIt(t *testing.T) {
	css := appStylesheet(t)
	if !regexp.MustCompile(`\.sidebar\{[^}]*position:fixed`).MatchString(css) {
		t.Skip("the sidebar is no longer fixed; the fullPage hazard this guards is gone")
	}

	script := repoScript(t, "ui-check.mjs")
	start := strings.Index(script, "async function shoot(")
	if start < 0 {
		t.Fatal("scripts/ui-check.mjs has no shoot(); this test cannot tell how the " +
			"screenshots are captured")
	}
	end := strings.Index(script[start:], "\n}\n")
	if end < 0 {
		t.Fatal("could not find the end of shoot()")
	}
	body := script[start : start+end]

	if strings.Contains(body, "fullPage") {
		t.Error("shoot() captures with fullPage while .sidebar is position: fixed\n" +
			"  the sidebar stops at the viewport height and its footer lands in the " +
			"middle of every screenshot taller than the window")
	}
	if !strings.Contains(body, "setViewportSize") {
		t.Error("shoot() no longer grows the viewport before capturing\n" +
			"  without it only the top of a long page is in the image")
	}
}
