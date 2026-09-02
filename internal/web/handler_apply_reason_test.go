package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The apply page must not say "the window closed without a confirmation" to
// the operator who just closed it themselves by clicking Roll back now.
// AcceptanceReason is how the template tells the two apart; this pins it
// against the literal token the core (and the demo) actually write.
//
// Both leads' full text is also embedded once, verbatim, in the per-page
// client-strings blob (clientStringKeys) so app.js can render either one after
// a poll — so the assertions below must look inside the rendered
// <p class="apply-lead"> element specifically, not anywhere in the body; a
// plain substring check against the whole page would find both sentences on
// every render and prove nothing.
func TestHandleApplyGET_RolledBackByOperatorGetsTheOperatorLead(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{
		Acceptance:       shared.AcceptanceRolledBack,
		AcceptanceReason: "cancelled by operator",
	}))

	rec := doAuthRequest(t, s, "GET", "/apply", nil)
	assertStatus(t, rec, http.StatusOK)
	lead := applyLeadText(t, rec.Body.String())

	if !strings.Contains(lead, "You rolled the change back") {
		t.Errorf("an operator rollback did not get the operator lead; .apply-lead:\n%s", lead)
	}
	if strings.Contains(lead, "The window closed without a confirmation") {
		t.Error("an operator rollback still shows the timeout lead — the operator clicked " +
			"the button, nothing timed out")
	}
}

// A rollback the operator did not ask for — the window simply running out —
// must keep the wording that was already correct for it.
func TestHandleApplyGET_RolledBackByTimeoutKeepsTheTimeoutLead(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{
		Acceptance:       shared.AcceptanceRolledBack,
		AcceptanceReason: "timeout",
	}))

	rec := doAuthRequest(t, s, "GET", "/apply", nil)
	assertStatus(t, rec, http.StatusOK)
	lead := applyLeadText(t, rec.Body.String())

	if !strings.Contains(lead, "The window closed without a confirmation") {
		t.Errorf("a timeout rollback lost its own lead; .apply-lead:\n%s", lead)
	}
	if strings.Contains(lead, "You rolled the change back") {
		t.Error("a timeout rollback shows the operator lead — nobody clicked anything")
	}
}

// applyLeadText returns the contents of the rendered <p class="apply-lead">
// element, so a check against it cannot be satisfied by the client-strings
// blob that carries both sentences on every render.
func applyLeadText(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, `class="apply-lead"`)
	if start < 0 {
		t.Fatalf("no .apply-lead element in the rendered page:\n%s", body)
	}
	rest := body[start:]
	open := strings.Index(rest, ">")
	end := strings.Index(rest, "</p>")
	if open < 0 || end < 0 || end < open {
		t.Fatalf("could not read the .apply-lead element's contents:\n%s", body)
	}
	return rest[open+1 : end]
}
