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

	// PendingCount is how many changes the Unapplied changes chip is about. Zero
	// when nothing is pending, and the chip is not shown then either.
	PendingCount int
}

// recentActivityLimit is how many audit entries the dashboard shows. Enough to
// answer "what changed here lately" without turning into a second log page.
const recentActivityLimit = 6

// countListEntries counts what is actually a rule.
//
// The three free-text lists keep the operator's `#` comments and the blank lines
// between groups, and every other counter in the interface skips them — the
// template helper the editors use says so in as many words. The dashboard tiles
// took a plain len() instead, so the same list was "12 entries" on one page and
// "7" on another, and the number that disagreed was the one on the front page.
// The demo ships lists full of comments, so it was visible to anyone who looked.
func countListEntries(lines []string) int {
	n := 0
	for _, l := range lines {
		if !shared.IsListComment(l) {
			n++
		}
	}
	return n
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
			TCP:        len(rules.Current.TCP),
			UDP:        len(rules.Current.UDP),
			Blacklist:  countListEntries(rules.Current.Blacklist),
			Whitelist:  countListEntries(rules.Current.Whitelist),
			Custom:     countListEntries(rules.Current.Custom),
			Forwarding: len(rules.Current.Forwarding),
		}
	}

	// Only when something is pending, so a quiet dashboard costs no extra reads:
	// three more round trips over a local socket on the load where the number is
	// worth having, and none on the load where it is zero.
	if status != nil && status.HasPending {
		data.PendingCount = s.pendingChangeCount()
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

// pendingChangeCount is the number the dashboard's chip carries: rule deltas
// plus configuration drift, the same two halves /apply lists. It is a count of
// changes and not of pages, so an option and a port are one each.
func (s *Server) pendingChangeCount() int {
	state, err := s.client.GetRules()
	if err != nil {
		return 0
	}
	n := len(shared.DiffRules(state.Current, state.Staged))

	opts, oErr := s.client.GetOptions()
	nets, nErr := s.client.GetSettings()
	applied, aErr := s.client.GetAppliedConfig()
	if oErr == nil && nErr == nil && aErr == nil && applied.Recorded {
		n += len(shared.DiffConfig(applied.Config, shared.AppliedConfig{
			Firewall: *opts, Network: *nets,
		}))
	}
	return n
}
