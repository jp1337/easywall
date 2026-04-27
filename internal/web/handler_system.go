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

	dur, err := strconv.Atoi(r.FormValue("acceptance_duration"))
	if err != nil || dur <= 0 {
		s.setFlash(w, r, "system_invalid_duration")
		http.Redirect(w, r, "/system", http.StatusSeeOther)
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
	}

	s.setFlash(w, r, "system_saved")
	http.Redirect(w, r, "/system", http.StatusSeeOther)
}
