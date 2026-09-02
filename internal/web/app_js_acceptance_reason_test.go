package web

import (
	"strings"
	"testing"
)

// Before 2.14 a timeout was the only way a window could end, and the poll's
// rolled_back branch announced "acceptance timeout" unconditionally. Clicking
// Roll back now produces the identical acceptance status, so it kept saying
// the same false thing about a rollback the operator had just caused
// themselves. The branch has to read data.acceptance_reason and choose between
// the two toast keys, not hard-code the timeout one.
func TestAppJS_RolledBackToastSelectsByReason(t *testing.T) {
	src := appJS(t)
	status := section(t, src, "function initApplyStatus")

	if !strings.Contains(status, "data.acceptance_reason") {
		t.Error("render() never reads data.acceptance_reason, so it cannot tell an operator " +
			"rollback from a timeout and always shows the timeout toast")
	}
	if !strings.Contains(status, "apply_rolled_back_operator_toast") {
		t.Error("render() never mentions apply_rolled_back_operator_toast, so an operator " +
			"rollback still gets the timeout wording")
	}
	if !strings.Contains(status, "apply_rolled_back_toast") {
		t.Error("render() lost the timeout toast key entirely; a genuine timeout must keep " +
			"the wording that was already correct for it")
	}
}

// apply.html renders the lead paragraph once, from the status the page loaded
// with. Watching an open window run to its natural end without navigating
// left it reading the pending sentence beside a dot the same poll had already
// changed to "Rolled back" — render() has to update .apply-lead itself, not
// only the dot above it.
func TestAppJS_RolledBackUpdatesTheLeadParagraph(t *testing.T) {
	src := appJS(t)
	status := section(t, src, "function initApplyStatus")

	if !strings.Contains(status, "apply-lead") {
		t.Error("render() never touches .apply-lead, so a window watched to its natural end " +
			"leaves the server's stale sentence on screen next to a dot that already changed")
	}
	if !strings.Contains(status, "apply_lead_rolled_back_operator") {
		t.Error("render() never mentions apply_lead_rolled_back_operator, so it cannot pick " +
			"the operator lead when updating .apply-lead")
	}
}

// Both new strings are rendered by app.js in the browser, so they must be in
// the blob every page inlines — or they render as empty text in every
// language, including English. Modeled on
// TestClientStringsCoverTheChipsEndStates.
func TestClientStringsCoverTheAcceptanceReasonStrings(t *testing.T) {
	for _, key := range []string{
		"apply_rolled_back_operator_toast",
		"apply_lead_rolled_back_operator",
		"apply_lead_rolled_back",
	} {
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
