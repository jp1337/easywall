package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The strings app.js builds in the browser have to be in the blob that is
// inlined into every page, or the chip's end state renders as an empty span in
// every language including English.
func TestClientStringsCoverTheChipsEndStates(t *testing.T) {
	for _, key := range []string{"apply_chip_confirmed", "apply_chip_rolled_back"} {
		found := false
		for _, k := range clientStringKeys {
			if k == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%q is not in clientStringKeys, so app.js cannot render it", key)
		}
	}
}

// One request per window from a page that is not /apply, and it is made at
// zero. A setInterval poll on every page would multiply the socket cost this
// release just spent a TTL cache reducing.
func TestAppJS_TheChipDoesNotPoll(t *testing.T) {
	src := appJS(t)

	chip := section(t, src, "Acceptance chip")
	if strings.Contains(chip, "setInterval") {
		t.Error("the topbar chip polls on an interval. It renders the remaining seconds from " +
			"the page load, ticks locally, and makes exactly one fetch at zero to learn which " +
			"way the window ended — that is what makes it free on every page")
	}
	if !strings.Contains(chip, "/apply/status") {
		t.Error("the chip never asks what happened, so it can only guess at zero")
	}
}

// The chip's starting number is a cached GET_STATUS up to statusTTL (2s) old,
// so its local clock routinely reaches zero before the window the core is
// tracking actually ends. A poll landing on pending must resume ticking, not
// read as a confirmation — found by inspection after this chip started
// appearing on every page.
func TestAppJS_ChipDoesNotConfirmAPendingWindow(t *testing.T) {
	src := appJS(t)
	chip := section(t, src, "Acceptance chip")
	if !strings.Contains(chip, "'pending'") {
		t.Error("settle() no longer branches on the window still being pending; a chip whose " +
			"local clock reaches zero before the cached status catches up would announce a " +
			"still-open window as confirmed")
	}
}

// A failed request is not a rollback (that was already true) and it is not a
// confirmation either — the chip must say nothing definite when it cannot
// tell what happened, the same way initApplyStatus treats an unreachable core.
func TestAppJS_ChipDoesNotConfirmWhenItCannotTellWhatHappened(t *testing.T) {
	src := appJS(t)
	chip := section(t, src, "Acceptance chip")
	if !strings.Contains(chip, "state_unknown") {
		t.Error("settle() has no unknown outcome; a failed request or an unparseable response " +
			"falls into the same branch as a real confirmation and announces something that " +
			"has not happened")
	}
}

// At zero the screen waits rather than guesses. The local tick reaches 00:00 up
// to two seconds before the poll learns what actually happened, and the two
// outcomes — confirmed in the last instant, or rolled back — are not
// distinguishable from the browser.
func TestAppJS_TheCountdownStopsAtZeroAndClaimsNothing(t *testing.T) {
	src := appJS(t)
	countdown := section(t, src, "Acceptance countdown")

	if !strings.Contains(countdown, "Math.max(0") {
		t.Error("the countdown does not clamp at zero; it renders negative time on the one " +
			"screen whose argument is that it says what is true")
	}
	for _, forbidden := range []string{"state_rolled_back", "apply_rolled_back_toast"} {
		if strings.Contains(countdown, forbidden) {
			t.Errorf("the countdown announces %q on its own. It must never report an outcome "+
				"the core has not reported: the state word changes when a poll returns "+
				"something else, and not before", forbidden)
		}
	}
}

// The pending dot must keep the class that stops it pulsing beside a running
// countdown (DESIGN.md, amended in 2.14: two "still happening" indicators side
// by side is one too many) when the poll rewrites the element two seconds
// after page load. Losing it here silently defeats that amendment.
func TestAppJS_PendingDotStaysStatic(t *testing.T) {
	src := appJS(t)
	status := section(t, src, "function initApplyStatus")
	if !strings.Contains(status, "'pending static'") {
		t.Error("the pending state's class no longer carries 'static'; the first poll after " +
			"page load rewrites the dot without it and it starts pulsing again")
	}
}

// #rollback-btn must be hidden and shown exactly like #confirm-btn, in both
// branches of render — otherwise a page rendered during a window whose poll
// later observes accepted or rolled_back leaves "Roll back now" on screen with
// nothing left to cancel.
func TestAppJS_RollbackButtonIsToggled(t *testing.T) {
	src := appJS(t)
	status := section(t, src, "function initApplyStatus")
	if !strings.Contains(status, "rollback-btn") {
		t.Error("render() never looks up #rollback-btn, so it is never hidden when the window ends")
	}
	if strings.Count(status, "rollbackBtn.toggleAttribute") < 2 {
		t.Error("#rollback-btn must be toggled in both the !known branch and the pending branch, " +
			"exactly like #confirm-btn — offering it for a window nobody knows the state of, or " +
			"one that has already closed, is a guess this screen must not make")
	}
}

func appJS(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if data, err := os.ReadFile(filepath.Join(dir, "web", "static", "app.js")); err == nil { // #nosec G304
			return string(data)
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not locate web/static/app.js")
	return ""
}

// section returns the body of one banner-commented block in app.js. A heading
// that matches nothing fails: a guard reading an empty string passes everything.
func section(t *testing.T, src, heading string) string {
	t.Helper()
	start := strings.Index(src, heading)
	if start < 0 {
		t.Fatalf("no %q section in app.js; this guard is reading nothing and would pass "+
			"whatever the file contained", heading)
	}
	rest := src[start:]
	if end := strings.Index(rest, "\n/* ── "); end > 0 {
		return rest[:end]
	}
	return rest
}
