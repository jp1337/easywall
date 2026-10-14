package web

import (
	"regexp"
	"strings"
	"testing"
)

// The tail asks for what is new (after=<seq>, 2.25.2) and skips the poll while
// paused, or while the pointer or focus is in the table — the fix for F3 and
// F12. An in-flight poll must not swap into a paused table (htmx:beforeSwap),
// and the pointer counts only on devices that hover. The behaviour itself is
// proven in a browser by ui-check's checkBlockedTailHoldsStill; this keeps the
// pieces from being deleted in a refactor that the browser check would only
// catch at the end.
func TestAppJS_TheBlockedTailPausesWhileTheTableIsInUse(t *testing.T) {
	src := appJS(t)
	tail := section(t, src, "Blocked traffic live tail")
	for _, want := range []string{
		"htmx:beforeRequest", "blocked-rows", ":hover", "document.activeElement", "preventDefault",
		"htmx:configRequest", "parameters.after", "htmx:afterSwap", "HX-Reswap", "dataset.limit", "pkt-new",
		"blocked-live-toggle",
		"htmx:beforeSwap", "shouldSwap", "(hover: hover)", "hadRows",
		// Read per check, not once at load: a tablet gains a mouse.
		"hoverQuery.matches",
		// A live region is silent when set to the text it already holds; it
		// is emptied first so a second "1 new entry" is announced.
		"announce.textContent = ''",
	} {
		if !strings.Contains(tail, want) {
			t.Errorf("the live-tail section does not contain %q", want)
		}
	}
	// A toggle button whose label changes must not also carry aria-pressed
	// (ARIA Authoring Practices, Button): the label already says Pause or Resume.
	if strings.Contains(tail, "aria-pressed") {
		t.Error("the live-tail section sets aria-pressed on a button whose label changes")
	}
	if !regexp.MustCompile(`(?m)^\s*initBlockedTail\(\);`).MatchString(src) {
		t.Error("initBlockedTail is never called")
	}
}
