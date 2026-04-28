package web

import (
	"log/slog"
	"net/http"

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
