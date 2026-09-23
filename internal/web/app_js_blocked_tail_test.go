package web

import (
	"regexp"
	"strings"
	"testing"
)

// The tail replaces every row on each poll. Skipping the poll while the
// pointer or focus is in the table is the whole fix for F3 and F12; the
// behaviour itself is proven in a browser by ui-check's checkBlockedTailHoldsStill.
func TestAppJS_TheBlockedTailPausesWhileTheTableIsInUse(t *testing.T) {
	src := appJS(t)
	tail := section(t, src, "Blocked traffic live tail")
	for _, want := range []string{"htmx:beforeRequest", "blocked-rows", ":hover", "document.activeElement", "preventDefault"} {
		if !strings.Contains(tail, want) {
			t.Errorf("the live-tail section does not contain %q", want)
		}
	}
	if !regexp.MustCompile(`(?m)^\s*initBlockedTail\(\);`).MatchString(src) {
		t.Error("initBlockedTail is never called")
	}
}
