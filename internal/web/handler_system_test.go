package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

// ── HTMX path: respondPartialSave / respondPartialError ──────────────────────

// doAuthFormHTMX performs an authenticated POST with the HX-Request header
// set so the server takes the partial-save code path.
func doAuthFormHTMX(t *testing.T, s *Server, url, formBody string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", url, strings.NewReader(formBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(makeAuthCookie(t, s.store))
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

func TestHandleSystemPOST_HTMX_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doAuthFormHTMX(t, s, "/system", "acceptance_enabled=on&acceptance_duration=60")
	assertStatus(t, rec, http.StatusNoContent)
	trigger := rec.Header().Get("HX-Trigger")
	if !strings.Contains(trigger, "easywall:saved") {
		t.Errorf("expected HX-Trigger easywall:saved, got %q", trigger)
	}
	if !strings.Contains(trigger, "system_saved") {
		t.Errorf("expected flash key 'system_saved' in trigger, got %q", trigger)
	}
}

func TestHandleSystemPOST_HTMX_InvalidDuration(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doAuthFormHTMX(t, s, "/system", "acceptance_duration=0")
	assertStatus(t, rec, http.StatusOK)
	trigger := rec.Header().Get("HX-Trigger")
	if !strings.Contains(trigger, "easywall:error") {
		t.Errorf("expected HX-Trigger easywall:error, got %q", trigger)
	}
	if !strings.Contains(trigger, "system_invalid_duration") {
		t.Errorf("expected flash key 'system_invalid_duration' in trigger, got %q", trigger)
	}
}

func TestHandleSystemPOST_HTMX_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveSystem, errorRespFor("save failed"))

	rec := doAuthFormHTMX(t, s, "/system", "acceptance_duration=120")
	assertStatus(t, rec, http.StatusOK)
	trigger := rec.Header().Get("HX-Trigger")
	if !strings.Contains(trigger, "easywall:error") {
		t.Errorf("expected HX-Trigger easywall:error, got %q", trigger)
	}
}
