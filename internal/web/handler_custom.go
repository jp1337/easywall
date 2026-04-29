package web

import (
	"log/slog"
	"net/http"
)

type customData struct {
	Rules  []string
	Errors map[int]string // line index → error message
}

func (s *Server) handleCustomGET(w http.ResponseWriter, r *http.Request) {
	state, err := s.client.GetRules()
	if err != nil {
		slog.Warn("get rules error", "error", err)
		s.render(w, r, "custom.html", "custom", &customData{Errors: nil})
		return
	}
	s.render(w, r, "custom.html", "custom", &customData{Rules: state.Staged.Custom, Errors: nil})
}

func (s *Server) handleCustomPOST(w http.ResponseWriter, r *http.Request) {
	rules := parseIPList(r.FormValue("rules")) // reuses newline-parser

	// Validate syntax via core
	if errs, err := s.client.ValidateCustom(rules); err != nil {
		slog.Warn("validate custom rules error", "error", err)
		// Core unavailable — skip validation, let apply catch errors
	} else if len(errs) > 0 {
		s.render(w, r, "custom.html", "custom", &customData{Rules: rules, Errors: errs})
		return
	}

	if err := s.client.SaveRules("custom", rules); err != nil {
		slog.Warn("save custom rules error", "error", err)
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, "/custom", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "saved")
	http.Redirect(w, r, "/custom", http.StatusSeeOther)
}
