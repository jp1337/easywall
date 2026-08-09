package web

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jp1337/easywall/internal/shared"
)

type systemData struct {
	Settings *shared.SystemSettings
	CoreErr  string
}

func (s *Server) handleSystemGET(w http.ResponseWriter, r *http.Request) {
	data := &systemData{}

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
