package web

import (
	"log/slog"
	"net/http"
)

func (s *Server) handleAllowlistGET(w http.ResponseWriter, r *http.Request) {
	state, err := s.client.GetRules()
	if err != nil {
		slog.Warn("get rules error", "error", err)
		s.render(w, r, "allowlist.html", "allowlist", &ipListData{Title: "allowlist"})
		return
	}
	s.render(w, r, "allowlist.html", "allowlist", &ipListData{
		Title:   "allowlist",
		Entries: state.Staged.Allowlist,
	})
}

func (s *Server) handleAllowlistPOST(w http.ResponseWriter, r *http.Request) {
	raw := r.FormValue("entries")

	// See handleBlocklistPOST: an unchecked entry here is worse, not better —
	// an allowlist entry that never becomes a rule silently withdraws access
	// the operator believes they granted.
	if errs := validateIPListEntries(raw); len(errs) > 0 {
		s.rejectIPList(w, r, "allowlist", raw, errs)
		return
	}

	if err := s.client.SaveRules("allowlist", parseIPList(raw)); err != nil {
		slog.Warn("save allowlist error", "error", err)
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, "/allowlist", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "saved")
	http.Redirect(w, r, "/allowlist", http.StatusSeeOther)
}
