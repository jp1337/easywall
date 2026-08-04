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

	// The label is resolved in the request's own language, because the point of
	// searching it is that it is what the operator can see. A German session
	// shows "Regeln zurückgenommen"; matching only the English label, or only
	// the stored "apply_rolledback", would both come back empty.
	loc := NewLocalizer(s.bundle, r, s.cfg.Language)
	tFunc := func(id string, args ...interface{}) string { return T(loc, id, args...) }

	var filtered []shared.AuditLogEntry
	for _, e := range entries {
		hay := strings.ToLower(strings.Join([]string{
			e.Action, actionLabel(tFunc, e.Action), e.RuleType, e.Detail, e.User,
		}, " "))
		if strings.Contains(hay, q) {
			filtered = append(filtered, e)
		}
	}
	s.renderPartial(w, r, "log_rows", filtered)
}
