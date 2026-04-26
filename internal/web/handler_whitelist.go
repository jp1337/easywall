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
	entries := parseIPList(r.FormValue("entries"))

	if err := s.client.SaveRules("whitelist", entries); err != nil {
		slog.Warn("save whitelist error", "error", err)
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, "/whitelist", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "saved")
	http.Redirect(w, r, "/whitelist", http.StatusSeeOther)
}
