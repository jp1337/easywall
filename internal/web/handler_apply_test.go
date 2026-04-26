package web

import (
	"encoding/json"
	"net/http"
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
	fc.SetResponse(shared.CmdAccept, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/apply/confirm", "")
	assertRedirect(t, rec, "/dashboard")
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
