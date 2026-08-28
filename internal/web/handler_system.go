package web

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jp1337/easywall/internal/shared"
)

type systemData struct {
	Settings  *shared.SystemSettings
	CoreErr   string
	Telemetry bool

	// TelemetryEndpoint is shown on the page. "A random identifier and the
	// version" is only a checkable claim if the operator can also see where it
	// goes, and being told that in the interface beats having to find the
	// documentation.
	TelemetryEndpoint string

	// TelemetryProv is the marker beside the consent toggle, or nil when
	// EASYWALL_WEB_TELEMETRY is unset — which is every installation that does
	// not set it, and why the template guards with {{with}}.
	TelemetryProv *provenanceView
}

func (s *Server) handleSystemGET(w http.ResponseWriter, r *http.Request) {
	data := &systemData{
		Telemetry:         s.cfg.TelemetryEnabled(),
		TelemetryEndpoint: shared.TelemetryEndpoint,
		TelemetryProv:     s.provenanceFor("telemetry"),
	}

	settings, err := s.client.GetSystem()
	if err != nil {
		slog.Warn("could not get system settings", "error", err)
		data.CoreErr = err.Error()
	} else {
		data.Settings = settings
	}

	s.render(w, r, "system.html", "system", data)
}

func (s *Server) handleSystemPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// The number input carries min and max attributes, but those are a courtesy
	// to the browser: a form posted any other way ignores them. A one-second
	// window rolls back before the confirmation page can be read, which leaves
	// the firewall unchangeable through the interface.
	dur, err := strconv.Atoi(r.FormValue("acceptance_duration"))
	if err != nil || !shared.ValidAcceptanceDuration(dur) {
		s.respondPartialError(w, r, "/system", "system_invalid_duration")
		return
	}

	settings := shared.SystemSettings{
		Acceptance: shared.AcceptanceConfig{
			Enabled:  r.FormValue("acceptance_enabled") == "on",
			Duration: dur,
		},
	}

	if err := s.client.SaveSystem(settings); err != nil {
		slog.Warn("could not save system settings", "error", err)
		s.respondPartialError(w, r, "/system", "save_error")
		return
	}

	s.respondPartialSave(w, r, "/system", "system_saved")
}

// handleTelemetryPOST records the answer to being counted.
//
// Its own route, and deliberately not part of the settings save above: that one
// goes through the core, and consent that can only be withdrawn while another
// process is reachable is not consent. This writes web.toml and nothing else.
func (s *Server) handleTelemetryPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// The reset button is a second submit inside the same form, so it arrives
	// here rather than through a route of its own. It clears the stored answer;
	// the checkbox's value is irrelevant to it and is deliberately not read.
	if r.FormValue("reset") != "" {
		if err := s.cfg.ResetTelemetry(); err != nil {
			slog.Warn("could not reset the telemetry setting", "error", err)
			s.respondPartialError(w, r, "/system", "save_error")
			return
		}
		slog.Info("telemetry setting reset to the environment")

		if !isHTMX(r) {
			s.respondPartialSave(w, r, "/system", "provenance_reset_done")
			return
		}

		// respondPartialSave's usual 204 leaves htmx with nothing to swap, and
		// a reset changes what is true on screen, not just what is stored: the
		// environment's value is now in force, so the checkbox may no longer
		// match what it displayed a moment ago, and the button the operator
		// just clicked must disappear — there is nothing left for it to
		// reset. This release exists to stop the interface asserting a
		// provenance it does not have; leaving the just-superseded state on
		// screen after the very action that superseded it would be that same
		// mistake. hx-swap="none" only suppresses the main response target;
		// htmx still applies hx-swap-oob elements found in the body, which is
		// how the checkbox and marker get back in sync without a page load.
		w.Header().Set("HX-Trigger", `{"easywall:saved":"provenance_reset_done"}`)
		s.renderPartial(w, r, "telemetry_state_oob", &systemData{
			Telemetry:     s.cfg.TelemetryEnabled(),
			TelemetryProv: s.provenanceFor("telemetry"),
		})
		return
	}

	enabled := r.FormValue("telemetry") == "on"
	if err := s.cfg.SaveTelemetry(enabled); err != nil {
		slog.Warn("could not save the telemetry setting", "error", err)
		s.respondPartialError(w, r, "/system", "save_error")
		return
	}

	// The reporter reads consent through the config on every attempt, so there
	// is nothing to restart: switching off here means the next attempt sends
	// nothing.
	slog.Info("telemetry setting changed", "enabled", enabled)
	s.respondPartialSave(w, r, "/system", "system_saved")
}
