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

// The number input advertises min=10 and max=3600. That is a browser hint, and
// the handler took any positive number — so a one-second window, which rolls
// back before the confirmation page can even be read, went through and made the
// firewall unchangeable through the interface.
func TestHandleSystemPOST_RejectsADurationOutsideTheAdvertisedRange(t *testing.T) {
	for _, dur := range []string{"0", "1", "9", "3601", "86400", "-5"} {
		t.Run(dur, func(t *testing.T) {
			fc := newFakeCore(t)
			s := newTestServer(t, fc)
			fc.SetResponse(shared.CmdSaveSystem, shared.Response{Success: true})

			rec := doAuthFormRequest(t, s, "/system",
				"acceptance_enabled=on&acceptance_duration="+dur)
			assertRedirect(t, rec, "/system")

			if cmd := fc.LastCommand(); cmd != nil {
				t.Errorf("duration %s must not reach the core, got command %q", dur, cmd.Type)
			}
		})
	}
}

func TestHandleSystemPOST_AcceptsTheRangeBoundaries(t *testing.T) {
	for _, dur := range []string{"10", "3600"} {
		t.Run(dur, func(t *testing.T) {
			fc := newFakeCore(t)
			s := newTestServer(t, fc)
			fc.SetResponse(shared.CmdSaveSystem, shared.Response{Success: true})

			rec := doAuthFormRequest(t, s, "/system",
				"acceptance_enabled=on&acceptance_duration="+dur)
			assertRedirect(t, rec, "/system")

			cmd := fc.LastCommand()
			if cmd == nil || cmd.Type != shared.CmdSaveSystem {
				t.Errorf("duration %s is permitted and must be saved, got %v", dur, cmd)
			}
		})
	}
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
	req.AddCookie(makeAuthCookie(t, s))
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

// ── Telemetry ───────────────────────────────────────────────────────────────

// Withdrawing consent goes through its own route on purpose: the settings on
// the same page travel through the core, and a core that cannot be reached
// must not be able to keep an installation counted.
func TestHandleTelemetryPOST_WorksWithoutTheCore(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	if err := s.cfg.SaveTelemetry(true); err != nil {
		t.Fatal(err)
	}
	fc.listener.Close() // the core is gone from here on

	rec := doFormRequest(s, "POST", "/system/telemetry", "", makeAuthCookie(t, s))
	if rec.Code >= 400 {
		t.Fatalf("consent could not be withdrawn: HTTP %d", rec.Code)
	}
	if s.cfg.TelemetryEnabled() {
		t.Error("the installation is still counted after the switch was turned off")
	}
}

func TestHandleTelemetryPOST_RecordsConsent(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	doFormRequest(s, "POST", "/system/telemetry", "telemetry=on", makeAuthCookie(t, s))
	if !s.cfg.TelemetryEnabled() {
		t.Error("switching it on was not recorded")
	}

	doFormRequest(s, "POST", "/system/telemetry", "", makeAuthCookie(t, s))
	if s.cfg.TelemetryEnabled() {
		t.Error("switching it off was not recorded")
	}
}

// The public demo is wiped every few hours, identifier included. Left counting,
// it would invent several installations a day in a number whose entire value is
// being small enough to mean something.
func TestNewServer_DemoModeNeverCounts(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	s.cfg.DemoMode = true

	demo, err := NewServer(s.cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer demo.Stop()

	if demo.telemetry != nil {
		t.Error("the demo would report itself as an installation")
	}
}

// The System page has to show what is sent and where — a claim about outbound
// traffic that the interface itself does not state is one an operator has to
// take on faith.
func TestHandleSystemGET_NamesTheTelemetryEndpoint(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/system", nil, makeAuthCookie(t, s))
	body := rec.Body.String()
	if !strings.Contains(body, shared.TelemetryEndpoint) {
		t.Errorf("the System page does not say where reports go (%q)", shared.TelemetryEndpoint)
	}
	if !strings.Contains(body, `action="/system/telemetry"`) {
		t.Error("the telemetry switch is not on its own form; a core outage would block it")
	}
}
