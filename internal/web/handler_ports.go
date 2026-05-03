package web

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jp1337/easywall/internal/shared"
)

type portsData struct {
	RuleType string // "tcp" or "udp"
	Rules    []shared.PortRule
}

func (s *Server) handlePortsGET(w http.ResponseWriter, r *http.Request) {
	ruleType := r.URL.Query().Get("type")
	if ruleType != "udp" {
		ruleType = "tcp"
	}

	state, err := s.client.GetRules()
	if err != nil {
		slog.Warn("get rules error", "error", err)
		s.render(w, r, "ports.html", "ports", &portsData{RuleType: ruleType})
		return
	}

	rules := state.Staged.TCP
	if ruleType == "udp" {
		rules = state.Staged.UDP
	}
	s.render(w, r, "ports.html", "ports", &portsData{RuleType: ruleType, Rules: rules})
}

func (s *Server) handlePortsPOST(w http.ResponseWriter, r *http.Request) {
	// Pick the redirect from a fixed allow-list of two literal URLs so
	// taint analysis can verify there is no open-redirect path. The
	// branch on r.FormValue("type") is a value comparison, not used to
	// construct the URL — gosec G710 won't flag this shape.
	ruleType := "tcp"
	redirect := "/ports?type=tcp"
	if r.FormValue("type") == "udp" {
		ruleType = "udp"
		redirect = "/ports?type=udp"
	}

	rulesJSON := r.FormValue("rules")
	var rules []shared.PortRule
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		s.setFlash(w, r, "invalid_rules")
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}

	if err := s.client.SaveRules(ruleType, rules); err != nil {
		slog.Warn("save rules error", "type", ruleType, "error", err)
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "saved")
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}
