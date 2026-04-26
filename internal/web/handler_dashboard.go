package web

import (
	"log/slog"
	"net/http"

	"github.com/jp1337/easywall/internal/shared"
)

type dashboardData struct {
	Status  *shared.FirewallStatus
	Version *shared.VersionInfo
	CoreErr string
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	data := &dashboardData{}

	status, err := s.client.GetStatus()
	if err != nil {
		slog.Warn("could not get firewall status", "error", err)
		data.CoreErr = err.Error()
	} else {
		data.Status = status
	}

	// Version check (non-blocking — fail silently if unavailable)
	versionInfo, err := shared.CheckLatestVersion(s.cfg.VersionCachePath())
	if err != nil {
		slog.Debug("version check failed", "error", err)
	} else {
		data.Version = versionInfo
	}

	s.render(w, r, "dashboard.html", "dashboard", data)
}
