package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// askMarker is a string only the card renders.
const askMarker = `action="/telemetry/prompt"`

func withVersion(t *testing.T, v string) {
	t.Helper()
	prev := shared.CurrentVersion
	shared.CurrentVersion = v
	t.Cleanup(func() { shared.CurrentVersion = prev })
}

func dashboardAsks(t *testing.T, s *Server) bool {
	t.Helper()
	rec := doAuthRequest(t, s, "GET", "/dashboard", nil)
	assertStatus(t, rec, http.StatusOK)
	return strings.Contains(rec.Body.String(), askMarker)
}

func writePrompt(t *testing.T, s *Server, body string) {
	t.Helper()
	if err := os.WriteFile(s.cfg.TelemetryPromptPath(), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func readPrompt(t *testing.T, s *Server) telemetryPrompt {
	t.Helper()
	return readTelemetryPrompt(s.cfg.TelemetryPromptPath())
}

func promptServer(t *testing.T) *Server {
	t.Helper()
	withVersion(t, "2.28.0")
	s := newTestServer(t, newFakeCore(t))
	enrollFactor(t, s)
	return s
}

// D1: the card shows when every condition holds, and each one alone hides it.
func TestTelemetryPrompt_ShownOnlyWhenEveryConditionHolds(t *testing.T) {
	t.Run("never asked, off", func(t *testing.T) {
		if !dashboardAsks(t, promptServer(t)) {
			t.Error("an installation that was never asked is not asked")
		}
	})
	t.Run("stored no from an earlier minor", func(t *testing.T) {
		s := promptServer(t)
		if err := s.cfg.SaveTelemetry(false); err != nil {
			t.Fatal(err)
		}
		writePrompt(t, s, `{"asked":"2.27"}`)
		if !dashboardAsks(t, s) {
			t.Error("a no given on 2.27 is not asked again on 2.28")
		}
	})
	t.Run("counting on", func(t *testing.T) {
		s := promptServer(t)
		if err := s.cfg.SaveTelemetry(true); err != nil {
			t.Fatal(err)
		}
		if dashboardAsks(t, s) {
			t.Error("asked although counting is already on")
		}
	})
	t.Run("answered this minor", func(t *testing.T) {
		s := promptServer(t)
		writePrompt(t, s, `{"asked":"2.28"}`)
		if dashboardAsks(t, s) {
			t.Error("asked twice on the same minor")
		}
	})
	t.Run("patch release of an answered minor", func(t *testing.T) {
		s := promptServer(t)
		writePrompt(t, s, `{"asked":"2.28"}`)
		shared.CurrentVersion = "2.28.1"
		if dashboardAsks(t, s) {
			t.Error("a patch release asked again")
		}
	})
	t.Run("never", func(t *testing.T) {
		s := promptServer(t)
		writePrompt(t, s, `{"asked":"2.20","never":true}`)
		if dashboardAsks(t, s) {
			t.Error("asked after don't ask again")
		}
	})
	t.Run("variable set", func(t *testing.T) {
		t.Setenv("EASYWALL_WEB_TELEMETRY", "false")
		if dashboardAsks(t, promptServer(t)) {
			t.Error("asked although the deployment set EASYWALL_WEB_TELEMETRY")
		}
	})
	t.Run("version does not parse", func(t *testing.T) {
		s := promptServer(t)
		writePrompt(t, s, `{"asked":"2.28"}`)
		shared.CurrentVersion = "dev"
		if dashboardAsks(t, s) {
			t.Error("a dev build asked")
		}
	})
	t.Run("demo", func(t *testing.T) {
		withVersion(t, "2.28.0")
		s := newDemoTestServer(t)
		if s.askTelemetry() {
			t.Error("the demo asks")
		}
	})
	t.Run("broken file reads as never asked", func(t *testing.T) {
		s := promptServer(t)
		writePrompt(t, s, `{not json`)
		if !dashboardAsks(t, s) {
			t.Error("an unreadable marker hid the question")
		}
	})
}

func TestTelemetryPrompt_Answers(t *testing.T) {
	t.Run("yes", func(t *testing.T) {
		s := promptServer(t)
		rec := doFormRequest(s, "POST", "/telemetry/prompt", "answer=yes", makeAuthCookie(t, s))
		assertRedirect(t, rec, "/")
		if !s.cfg.TelemetryEnabled() {
			t.Error("yes did not switch counting on")
		}
		if p := readPrompt(t, s); p.Asked != "2.28" || p.Never {
			t.Errorf("marker after yes = %+v, want asked 2.28", p)
		}
	})
	t.Run("no", func(t *testing.T) {
		s := promptServer(t)
		doFormRequest(s, "POST", "/telemetry/prompt", "answer=no", makeAuthCookie(t, s))
		if s.cfg.Telemetry == nil || *s.cfg.Telemetry {
			t.Error("no was not stored as false")
		}
		if p := readPrompt(t, s); p.Asked != "2.28" || p.Never {
			t.Errorf("marker after no = %+v, want asked 2.28", p)
		}
		if dashboardAsks(t, s) {
			t.Error("the card came back after no")
		}
	})
	t.Run("never leaves the setting alone", func(t *testing.T) {
		s := promptServer(t)
		doFormRequest(s, "POST", "/telemetry/prompt", "answer=never", makeAuthCookie(t, s))
		if s.cfg.Telemetry != nil {
			t.Error("never wrote a telemetry value")
		}
		if p := readPrompt(t, s); !p.Never {
			t.Errorf("marker after never = %+v, want never", p)
		}
		shared.CurrentVersion = "2.29.0"
		if dashboardAsks(t, s) {
			t.Error("the next minor asked after never")
		}
	})
	t.Run("htmx removes the card", func(t *testing.T) {
		s := promptServer(t)
		req := httptest.NewRequest("POST", "/telemetry/prompt", strings.NewReader("answer=no"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		req.AddCookie(makeAuthCookie(t, s))
		rec := httptest.NewRecorder()
		s.router.ServeHTTP(rec, req)
		// 200 with nothing, not 204: htmx swaps nothing on a 204.
		if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
			t.Errorf("got %d with %d bytes, want 200 with an empty body", rec.Code, rec.Body.Len())
		}
		if !strings.Contains(rec.Header().Get("HX-Trigger"), "telemetry_prompt_saved_no") {
			t.Errorf("HX-Trigger = %q", rec.Header().Get("HX-Trigger"))
		}
	})
	t.Run("bad answer writes nothing", func(t *testing.T) {
		s := promptServer(t)
		rec := doFormRequest(s, "POST", "/telemetry/prompt", "answer=maybe", makeAuthCookie(t, s))
		assertStatus(t, rec, http.StatusBadRequest)
		if s.cfg.Telemetry != nil {
			t.Error("a bad answer wrote a telemetry value")
		}
		if _, err := os.Stat(s.cfg.TelemetryPromptPath()); err == nil {
			t.Error("a bad answer wrote a marker")
		}
	})
	t.Run("signed out", func(t *testing.T) {
		s := promptServer(t)
		assertRedirect(t, doFormRequest(s, "POST", "/telemetry/prompt", "answer=yes"), "/login")
		if s.cfg.TelemetryEnabled() {
			t.Error("a signed-out request switched counting on")
		}
	})
	t.Run("cross-site", func(t *testing.T) {
		s := promptServer(t)
		req := httptest.NewRequest("POST", "/telemetry/prompt", strings.NewReader("answer=yes"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		req.AddCookie(makeAuthCookie(t, s))
		rec := httptest.NewRecorder()
		s.router.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden || s.cfg.TelemetryEnabled() {
			t.Errorf("cross-site answer: HTTP %d, enabled %v", rec.Code, s.cfg.TelemetryEnabled())
		}
	})
}

// D9: the consent is saved first, and only a saved consent leaves a marker.
func TestTelemetryPrompt_FailedWrites(t *testing.T) {
	t.Run("consent not saved, no marker", func(t *testing.T) {
		s := promptServer(t)
		s.cfg.configPath = filepath.Join(t.TempDir(), "missing", "web.toml")
		doFormRequest(s, "POST", "/telemetry/prompt", "answer=yes", makeAuthCookie(t, s))
		if _, err := os.Stat(s.cfg.TelemetryPromptPath()); err == nil {
			t.Error("a consent that failed to save left a marker")
		}
	})
	t.Run("marker not written, consent kept", func(t *testing.T) {
		s := promptServer(t)
		dir := filepath.Dir(s.cfg.TelemetryPromptPath())
		if err := os.Chmod(dir, 0500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0750) })
		if os.Geteuid() == 0 {
			t.Skip("root writes through 0500")
		}
		rec := doFormRequest(s, "POST", "/telemetry/prompt", "answer=yes", makeAuthCookie(t, s))
		assertRedirect(t, rec, "/")
		if !s.cfg.TelemetryEnabled() {
			t.Error("a failed marker write lost the consent")
		}
	})
}

// D4: the wizard and System are answers too; the reset is not.
func TestTelemetryPrompt_OtherAnswersRecordTheMinor(t *testing.T) {
	t.Run("system save", func(t *testing.T) {
		s := promptServer(t)
		doFormRequest(s, "POST", "/system/telemetry", "", makeAuthCookie(t, s))
		if p := readPrompt(t, s); p.Asked != "2.28" {
			t.Errorf("a System save left marker %+v", p)
		}
	})
	t.Run("system save keeps never", func(t *testing.T) {
		s := promptServer(t)
		doFormRequest(s, "POST", "/telemetry/prompt", "answer=never", makeAuthCookie(t, s))
		doFormRequest(s, "POST", "/system/telemetry", "", makeAuthCookie(t, s))
		if p := readPrompt(t, s); !p.Never {
			t.Errorf("a later System save cleared don't ask again: %+v", p)
		}
	})
	t.Run("system reset", func(t *testing.T) {
		t.Setenv("EASYWALL_WEB_TELEMETRY", "true")
		s := promptServer(t)
		doFormRequest(s, "POST", "/system/telemetry", "reset=1", makeAuthCookie(t, s))
		if _, err := os.Stat(s.cfg.TelemetryPromptPath()); err == nil {
			t.Error("the reset recorded an answer")
		}
	})
}
