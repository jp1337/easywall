package web

import (
	"log/slog"
	"net/http"

	"github.com/jp1337/easywall/internal/shared"
)

type optionsData struct {
	Options *shared.FirewallOptions
	CoreErr string
}

// handleOptions shows the current firewall protection options (read-only).
// Options are set in easywall.toml — changing them requires editing the file and restarting the core.
func (s *Server) handleOptions(w http.ResponseWriter, r *http.Request) {
	opts, err := s.client.GetOptions()
	if err != nil {
		slog.Warn("get options error", "error", err)
		s.render(w, r, "options.html", "options", &optionsData{CoreErr: err.Error()})
		return
	}
	s.render(w, r, "options.html", "options", &optionsData{Options: opts})
}
