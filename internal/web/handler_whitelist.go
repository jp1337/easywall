package web

import (
	"log/slog"
	"net/http"
)

func (s *Server) handleWhitelistGET(w http.ResponseWriter, r *http.Request) {
	state, err := s.client.GetRules()
	if err != nil {
		slog.Warn("get rules error", "error", err)
		s.render(w, r, "whitelist.html", "whitelist", &ipListData{Title: "whitelist"})
		return
	}
	s.render(w, r, "whitelist.html", "whitelist", &ipListData{
		Title:   "whitelist",
		Entries: state.Staged.Whitelist,
	})
}

func (s *Server) handleWhitelistPOST(w http.ResponseWriter, r *http.Request) {
	raw := r.FormValue("entries")

	// See handleBlacklistPOST: an unchecked entry here is worse, not better —
	// a whitelist entry that never becomes a rule silently withdraws access
	// the operator believes they granted.
	if errs := validateIPListEntries(raw); len(errs) > 0 {
		s.rejectIPList(w, r, "whitelist", raw, errs)
		return
	}

	if err := s.client.SaveRules("whitelist", parseIPList(raw)); err != nil {
		slog.Warn("save whitelist error", "error", err)
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, "/whitelist", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "saved")
	http.Redirect(w, r, "/whitelist", http.StatusSeeOther)
}
