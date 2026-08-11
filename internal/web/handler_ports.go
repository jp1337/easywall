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

	// Checked here as well as in the core, for the message. The core refuses an
	// incomplete rule and the answer arrived as "Failed to save changes. Check
	// core connection." — which names the wrong cause and sends the operator to
	// look at a daemon that is working. The browser used to hide this case by
	// dropping half-filled rows before submitting, so the row disappeared and
	// nothing was reported at all.
	set := shared.Rules{TCP: rules}
	if ruleType == "udp" {
		set = shared.Rules{UDP: rules}
	}
	if err := shared.ValidateRules(set); err != nil {
		slog.Info("rejected port rules", "type", ruleType, "error", err)
		// Re-rendered rather than redirected, so the rows that were rejected are
		// the rows still on the screen — which is what the message says, and a
		// redirect would have thrown the operator's typing away to prove it
		// wrong. Same shape as the custom rules editor.
		s.setFlash(w, r, "save_invalid_ports")
		s.render(w, r, "ports.html", "ports", &portsData{RuleType: ruleType, Rules: rules})
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
