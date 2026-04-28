package web

import (
	"net/http"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHandleSystemGET_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/system", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleSystemGET_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetSystem, successResp(shared.SystemSettings{
		Acceptance: shared.AcceptanceConfig{Enabled: true, Duration: 120},
	}))

	rec := doAuthRequest(t, s, "GET", "/system", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleSystemGET_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetSystem, errorRespFor("core unavailable"))

	rec := doAuthRequest(t, s, "GET", "/system", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleSystemPOST_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/system", "acceptance_duration=120")
	assertRedirect(t, rec, "/login")
}

func TestHandleSystemPOST_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveSystem, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/system",
		"acceptance_enabled=on&acceptance_duration=60")
	assertRedirect(t, rec, "/system")
}

func TestHandleSystemPOST_InvalidDuration(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doAuthFormRequest(t, s, "/system", "acceptance_duration=abc")
	assertRedirect(t, rec, "/system")
}

func TestHandleSystemPOST_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveSystem, errorRespFor("save failed"))

	rec := doAuthFormRequest(t, s, "/system", "acceptance_duration=120")
	assertRedirect(t, rec, "/system")
}
