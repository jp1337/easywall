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
