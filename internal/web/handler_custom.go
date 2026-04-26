package web

import (
	"log/slog"
	"net/http"
)

type customData struct {
	Rules []string
}

func (s *Server) handleCustomGET(w http.ResponseWriter, r *http.Request) {
	state, err := s.client.GetRules()
	if err != nil {
		slog.Warn("get rules error", "error", err)
		s.render(w, r, "custom.html", "custom", &customData{})
		return
	}
	s.render(w, r, "custom.html", "custom", &customData{Rules: state.Staged.Custom})
}

func (s *Server) handleCustomPOST(w http.ResponseWriter, r *http.Request) {
	rules := parseIPList(r.FormValue("rules")) // reuses newline-parser

	if err := s.client.SaveRules("custom", rules); err != nil {
		slog.Warn("save custom rules error", "error", err)
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, "/custom", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "saved")
	http.Redirect(w, r, "/custom", http.StatusSeeOther)
}
