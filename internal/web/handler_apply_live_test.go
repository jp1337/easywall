package web

import (
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// mm:ss with a leading zero, always. tnum holds the width; the leading zero
// holds the *character count*, or the block reflows by one glyph at 1:00 → 0:59.
// Carries to 60:00, which AcceptanceDurationMax = 3600 makes the longest there is.
func TestMMSS(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want string
	}{
		{0, "00:00"},
		{9, "00:09"},
		{59, "00:59"},
		{60, "01:00"},
		{119, "01:59"},
		{120, "02:00"},
		{3600, "60:00"},
		{-5, "00:00"}, // never a negative clock
	} {
		if got := mmss(tc.in); got != tc.want {
			t.Errorf("mmss(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// handleApplyGET set Preview to nil while a window was open — right about the
// preview, wrong about the operator. Step 2 tells them to check that SSH and
// their own services still answer, and what to check was then not on the page:
// only a count survived, and a count does not name a port.
func TestApplyGET_ListsWhatIsLiveDuringTheWindow(t *testing.T) {
	srv := newDemoTestServer(t)

	if err := srv.client.SaveRules("tcp", []shared.PortRule{{Port: "8443"}}); err != nil {
		t.Fatalf("SaveRules: %v", err)
	}
	if err := srv.client.ApplyRules(); err != nil {
		t.Fatalf("ApplyRules: %v", err)
	}

	rec := getAuthenticated(t, srv, "/apply")
	body := rec.Body.String()

	if !strings.Contains(body, "8443") {
		t.Error("the open window does not name the port it made live; the operator is told " +
			"to check their services and not told which one changed")
	}
	if !strings.Contains(body, `id="apply-countdown"`) {
		t.Error("no countdown element on a page with a window open")
	}
	if !strings.Contains(body, `action="/apply/rollback"`) {
		t.Error("no rollback form on a page with a window open; DESIGN.md requires both " +
			"actions offered plainly")
	}
	// state-warn throughout, never state-crit: a window with eight seconds left
	// is not rolled back, unreachable or invalid.
	if strings.Contains(body, "countdown state-crit") {
		t.Error("the countdown is rendered in state-crit; DESIGN.md rule 1 reserves that " +
			"for rolled back, unreachable and validation failed")
	}
}

func TestApplyGET_NoCountdownWhenNoWindowIsOpen(t *testing.T) {
	srv := newDemoTestServer(t)
	rec := getAuthenticated(t, srv, "/apply")
	body := rec.Body.String()
	if strings.Contains(body, `id="apply-countdown"`) {
		t.Error("a countdown is rendered with no window open")
	}
}
