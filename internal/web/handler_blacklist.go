package web

import (
	"log/slog"
	"net/http"
	"strings"
)

type ipListData struct {
	Title   string
	Entries []string
}

func (s *Server) handleBlacklistGET(w http.ResponseWriter, r *http.Request) {
	state, err := s.client.GetRules()
	if err != nil {
		slog.Warn("get rules error", "error", err)
		s.render(w, r, "blacklist.html", "blacklist", &ipListData{Title: "blacklist"})
		return
	}
	s.render(w, r, "blacklist.html", "blacklist", &ipListData{
		Title:   "blacklist",
		Entries: state.Staged.Blacklist,
	})
}

func (s *Server) handleBlacklistPOST(w http.ResponseWriter, r *http.Request) {
	raw := r.FormValue("entries")

	// The live validation beside the textarea is advisory — it swaps a message
	// into the page and does not stop the form. Until 2.5.0 nothing else
	// checked either, so a malformed address was saved, listed here as blocked,
	// and then silently skipped when the rules were applied. Refuse the save
	// instead, and say which line.
	if errs := validateIPListEntries(raw); len(errs) > 0 {
		slog.Info("rejected blacklist save", "invalid_lines", len(errs))
		s.setFlash(w, r, "save_invalid_entries")
		http.Redirect(w, r, "/blacklist", http.StatusSeeOther)
		return
	}

	if err := s.client.SaveRules("blacklist", parseIPList(raw)); err != nil {
		slog.Warn("save blacklist error", "error", err)
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, "/blacklist", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "saved")
	http.Redirect(w, r, "/blacklist", http.StatusSeeOther)
}

// parseIPList converts a newline-separated string into a slice of non-empty, trimmed entries.
func parseIPList(raw string) []string {
	var result []string
	for _, line := range strings.Split(raw, "\n") {
		entry := strings.TrimSpace(line)
		if entry != "" && !strings.HasPrefix(entry, "#") {
			result = append(result, entry)
		}
	}
	if result == nil {
		return []string{}
	}
	return result
}
