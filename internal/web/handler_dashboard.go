package web

import (
	"log/slog"
	"net/http"

	"github.com/jp1337/easywall/internal/shared"
)

type ruleCounts struct {
	TCP       int
	UDP       int
	Blacklist int
	Whitelist int
	Custom    int
}

type dashboardData struct {
	Status  *shared.FirewallStatus
	Version *shared.VersionInfo
	Counts  *ruleCounts
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

	if rules, err := s.client.GetRules(); err == nil {
		data.Counts = &ruleCounts{
			TCP:       len(rules.Current.TCP),
			UDP:       len(rules.Current.UDP),
			Blacklist: len(rules.Current.Blacklist),
			Whitelist: len(rules.Current.Whitelist),
			Custom:    len(rules.Current.Custom),
		}
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
