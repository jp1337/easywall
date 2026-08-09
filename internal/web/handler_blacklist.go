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

// parseIPList converts the textarea into the list that gets stored: one trimmed
// line per element, comments and the blank lines between groups kept.
//
// They used to be dropped here, which quietly deleted them. blacklist.md
// documents `#` comments, the entry counter is written to ignore them, the
// demo ships a list full of them, and the core skips them wherever it reads a
// list — every part of the system expected them to be there except the one
// function that decides what is saved. An operator who noted why an address was
// blocked lost the note on the next save, and the same applied to the hand
// written nftables statements on the custom rules page, where the comment is
// often the only thing explaining the rule.
//
// Trailing blank lines are dropped: they carry nothing and would otherwise
// accumulate at the end of the file on every save.
func parseIPList(raw string) []string {
	lines := strings.Split(raw, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		result = append(result, strings.TrimSpace(line))
	}
	for len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}
	return result
}
