package web

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/jp1337/easywall/internal/shared"
)

type logData struct {
	Entries []shared.AuditLogEntry
	CoreErr string
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	data := &logData{}

	entries, err := s.client.GetLog()
	if err != nil {
		slog.Warn("could not get audit log", "error", err)
		data.CoreErr = err.Error()
	} else {
		data.Entries = entries
	}

	s.render(w, r, "log.html", "log", data)
}

// handleLogFilter is an HTMX endpoint that returns just the filtered <tr>
// rows (the body of #log-rows). The query parameter `q` does a
// case-insensitive substring match against action, rule_type, detail, and
// user columns. An empty q returns all rows.
func (s *Server) handleLogFilter(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))

	entries, err := s.client.GetLog()
	if err != nil {
		slog.Warn("could not get audit log for filter", "error", err)
		// Render the empty-result row rather than 500ing — the user can
		// still see they're searching but the core is offline.
		s.renderPartial(w, r, "log_rows", []shared.AuditLogEntry{})
		return
	}

	if q == "" {
		s.renderPartial(w, r, "log_rows", entries)
		return
	}

	var filtered []shared.AuditLogEntry
	for _, e := range entries {
		// The displayed label is searched alongside the stored identifier. The
		// table shows "Rules rolled back" while the log holds
		// "apply_rolledback"; matching only the identifier meant typing what you
		// can see on screen returned nothing.
		hay := strings.ToLower(strings.Join([]string{
			e.Action, actionLabel(e.Action), e.RuleType, e.Detail, e.User,
		}, " "))
		if strings.Contains(hay, q) {
			filtered = append(filtered, e)
		}
	}
	s.renderPartial(w, r, "log_rows", filtered)
}
