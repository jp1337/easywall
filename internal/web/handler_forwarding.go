package web

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jpylypiw/easywall/internal/shared"
)

type forwardingData struct {
	Rules []shared.ForwardingRule
}

func (s *Server) handleForwardingGET(w http.ResponseWriter, r *http.Request) {
	state, err := s.client.GetRules()
	if err != nil {
		slog.Warn("get rules error", "error", err)
		s.render(w, r, "forwarding.html", "forwarding", &forwardingData{})
		return
	}
	s.render(w, r, "forwarding.html", "forwarding", &forwardingData{Rules: state.Staged.Forwarding})
}

func (s *Server) handleForwardingPOST(w http.ResponseWriter, r *http.Request) {
	rulesJSON := r.FormValue("rules")
	var rules []shared.ForwardingRule
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		s.setFlash(w, r, "invalid_rules")
		http.Redirect(w, r, "/forwarding", http.StatusSeeOther)
		return
	}

	if err := s.client.SaveRules("forwarding", rules); err != nil {
		slog.Warn("save forwarding error", "error", err)
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, "/forwarding", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "saved")
	http.Redirect(w, r, "/forwarding", http.StatusSeeOther)
}
