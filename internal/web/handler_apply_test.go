package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHandleApplyGET_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/apply", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleApplyGET_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{
		Active:     true,
		Acceptance: shared.AcceptancePending,
		HasPending: true,
	}))

	rec := doAuthRequest(t, s, "GET", "/apply", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleApplyGET_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetStatus, errorRespFor("status error"))

	rec := doAuthRequest(t, s, "GET", "/apply", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleApplyStart_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/apply/start", "")
	assertRedirect(t, rec, "/login")
}

func TestHandleApplyStart_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdApplyRules, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/apply/start", "")
	assertRedirect(t, rec, "/apply")
}

func TestHandleApplyStart_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdApplyRules, errorRespFor("apply failed"))

	rec := doAuthFormRequest(t, s, "/apply/start", "")
	assertRedirect(t, rec, "/apply")
}

func TestHandleApplyConfirm_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/apply/confirm", "")
	assertRedirect(t, rec, "/login")
}

func TestHandleApplyConfirm_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdAccept, successResp(shared.AcceptResult{Accepted: true}))

	rec := doAuthFormRequest(t, s, "/apply/confirm", "")
	assertRedirect(t, rec, "/dashboard")
}

// A confirmation that arrives after the window closed changes nothing — the
// rules were rolled back when it expired. The page used to say "Rules accepted
// and applied successfully" regardless and send the operator to the dashboard,
// telling them their change was live at the one moment it was not.
func TestHandleApplyConfirm_TooLateSaysSoInsteadOfClaimingSuccess(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdAccept, successResp(shared.AcceptResult{Accepted: false}))

	rec := doAuthFormRequest(t, s, "/apply/confirm", "")

	// Back to the apply page, where the real state is visible — not to the
	// dashboard with a success message.
	assertRedirect(t, rec, "/apply")

	cookie := rec.Result().Cookies()
	if len(cookie) == 0 {
		t.Fatal("expected a flash cookie explaining what happened")
	}
	follow := doRequest(s, "GET", "/apply", nil, cookie[0], makeAuthCookie(t, s))
	body := follow.Body.String()
	if strings.Contains(body, "accepted and applied successfully") {
		t.Error("the interface must not report success for a confirmation that came too late")
	}
	if !strings.Contains(body, "Too late") {
		t.Errorf("expected the page to say the window had closed; body was:\n%s", body)
	}
}

func TestHandleApplyConfirm_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdAccept, errorRespFor("accept error"))

	rec := doAuthFormRequest(t, s, "/apply/confirm", "")
	assertRedirect(t, rec, "/apply")
}

func TestHandleApplyStatus_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/apply/status", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleApplyStatus_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{
		Active:     true,
		Acceptance: shared.AcceptancePending,
		HasPending: true,
	}))

	rec := doAuthRequest(t, s, "GET", "/apply/status", nil)
	assertStatus(t, rec, http.StatusOK)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var status shared.FirewallStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if status.Acceptance != shared.AcceptancePending {
		t.Errorf("expected pending, got %s", status.Acceptance)
	}
}

func TestHandleApplyStatus_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetStatus, errorRespFor("status unavailable"))

	rec := doAuthRequest(t, s, "GET", "/apply/status", nil)
	assertStatus(t, rec, http.StatusServiceUnavailable)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
}

// An apply refused because a human took the firewall down at the console must
// say that, not report the generic "Failed to apply rules." — the same
// distinction handler_apply.go already draws for ErrApplyInProgressText.
// Mirroring shared.ErrPanicEngagedText here proves the recognition and gives
// the browser-visible text something other than a bare core error string.
func TestHandleApplyStart_PanicEngagedSaysSoInsteadOfGenericFailure(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdApplyRules, errorRespFor(shared.ErrPanicEngagedText))

	rec := doAuthFormRequest(t, s, "/apply/start", "")
	assertRedirect(t, rec, "/apply")

	cookie := rec.Result().Cookies()
	if len(cookie) == 0 {
		t.Fatal("expected a flash cookie explaining what happened")
	}
	follow := doRequest(s, "GET", "/apply", nil, cookie[0], makeAuthCookie(t, s))
	body := follow.Body.String()
	if strings.Contains(body, "Failed to apply rules") {
		t.Error("a refusal because panic mode is engaged must not read as a generic apply failure")
	}
	if !strings.Contains(body, "panic mode is engaged") {
		t.Errorf("expected the page to name panic mode as the reason; body was:\n%s", body)
	}
}

// The page names what changes, and it names it before the operator commits to
// finding out during the 120 seconds.
func TestHandleApplyGET_ListsWhatChanges(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{
		Active: true, Acceptance: shared.AcceptanceIdle, HasPending: true,
	}))
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Current: shared.Rules{TCP: []shared.PortRule{{Port: "22", Description: "SSH"}}},
		Staged: shared.Rules{TCP: []shared.PortRule{
			{Port: "22", Description: "SSH"},
			{Port: "8443", Description: "Nextcloud"},
		}},
	}))
	fc.SetResponse(shared.CmdGetOptions, successResp(shared.FirewallOptions{Fragments: true}))
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))
	fc.SetResponse(shared.CmdGetAppliedConfig, successResp(shared.AppliedConfigResult{
		Recorded: true,
		Config:   shared.AppliedConfig{Firewall: shared.FirewallOptions{Fragments: false}},
	}))

	rec := doAuthRequest(t, s, "GET", "/apply", nil)
	assertStatus(t, rec, http.StatusOK)
	body := rec.Body.String()

	for _, want := range []string{"8443", "Nextcloud", "drop_fragments"} {
		if !strings.Contains(body, want) {
			t.Errorf("the preview does not mention %q", want)
		}
	}
}

// The one number the operator has to be able to trust: what the verdict is about
// is the address the request actually came from, and the port this process
// listens on.
func TestHandleApplyGET_TheVerdictNamesTheOperatorsOwnAddress(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{HasPending: true}))
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Staged: shared.Rules{TCP: []shared.PortRule{{Port: "19999"}}},
	}))
	fc.SetResponse(shared.CmdGetOptions, successResp(shared.FirewallOptions{}))
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))
	fc.SetResponse(shared.CmdGetAppliedConfig, successResp(shared.AppliedConfigResult{}))

	rec := doAuthRequest(t, s, "GET", "/apply", nil)
	body := rec.Body.String()

	// httptest.NewRequest's peer is 192.0.2.1, and newTestServer binds :19999.
	if !strings.Contains(body, "192.0.2.1") || !strings.Contains(body, "19999") {
		t.Errorf("the verdict line does not name the address and port it is about:\n%s", body)
	}
}

// A window that is open is not a preview: the change is live already. The page
// says how much of it is, and shows no diff.
func TestHandleApplyGET_APendingWindowCountsWhatIsLive(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{
		Acceptance: shared.AcceptancePending, HasPending: false,
	}))
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Backup:  shared.Rules{TCP: []shared.PortRule{{Port: "22"}}},
		Current: shared.Rules{TCP: []shared.PortRule{{Port: "22"}, {Port: "8443"}}},
	}))

	rec := doAuthRequest(t, s, "GET", "/apply", nil)
	assertStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), `class="diff"`) {
		t.Error("a preview of a change that is already live is history, not a preview")
	}
}

// An installation that has not applied under 2.10 has no snapshot, and the page
// says so once rather than inventing a drift or hiding one.
func TestHandleApplyGET_AnUnrecordedSnapshotSaysSo(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{HasPending: true}))
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{}))
	fc.SetResponse(shared.CmdGetOptions, successResp(shared.FirewallOptions{}))
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))
	fc.SetResponse(shared.CmdGetAppliedConfig, successResp(shared.AppliedConfigResult{Recorded: false}))

	rec := doAuthRequest(t, s, "GET", "/apply", nil)
	if !strings.Contains(rec.Body.String(), "apply_config_unrecorded") &&
		!strings.Contains(rec.Body.String(), "not recorded") {
		t.Error("nothing on the page says the configuration that went in was never recorded")
	}
}
