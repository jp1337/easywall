package web

import (
	"log/slog"
	"net/http"

	"github.com/jp1337/easywall/internal/shared"
)

type ruleCounts struct {
	TCP        int
	UDP        int
	Blacklist  int
	Whitelist  int
	Custom     int
	Forwarding int
}

type dashboardData struct {
	Status  *shared.FirewallStatus
	Version *shared.VersionInfo
	Counts  *ruleCounts
	Recent  []shared.AuditLogEntry
	CoreErr string
}

// recentActivityLimit is how many audit entries the dashboard shows. Enough to
// answer "what changed here lately" without turning into a second log page.
const recentActivityLimit = 6

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
			TCP:        len(rules.Current.TCP),
			UDP:        len(rules.Current.UDP),
			Blacklist:  len(rules.Current.Blacklist),
			Whitelist:  len(rules.Current.Whitelist),
			Custom:     len(rules.Current.Custom),
			Forwarding: len(rules.Current.Forwarding),
		}
	}

	// Recent activity. Best-effort: the dashboard is still useful without it,
	// so a log read failure is logged and otherwise ignored rather than
	// surfaced as a page-level error.
	if entries, err := s.client.GetLog(); err != nil {
		slog.Debug("could not get audit log for dashboard", "error", err)
	} else if len(entries) > recentActivityLimit {
		data.Recent = entries[:recentActivityLimit]
	} else {
		data.Recent = entries
	}

	// Version check. Answered from cache; a stale cache is refreshed in the
	// background and shows up on the next load. The comment here used to say
	// "non-blocking" above a call that waited out a five-second HTTP timeout on
	// every render — on exactly the isolated hosts easywall is built for.
	data.Version = s.version.Info()

	s.render(w, r, "dashboard.html", "dashboard", data)
}
