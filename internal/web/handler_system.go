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
}

func (s *Server) handleSystemGET(w http.ResponseWriter, r *http.Request) {
	data := &systemData{
		Telemetry:         s.cfg.TelemetryEnabled(),
		TelemetryEndpoint: shared.TelemetryEndpoint,
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
