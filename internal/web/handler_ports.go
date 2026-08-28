package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jp1337/easywall/internal/shared"
)

type portsData struct {
	RuleType  string // "tcp" or "udp"
	Rules     []shared.PortRule
	Catalogue []catalogueEntry
}

// catalogueEntry is one service as the picker needs it: the rows it would add
// for *this* tab's protocol, and the suggested sources already joined into the
// string the field holds.
//
// Rendered into the page rather than fetched. There is no route because there is
// no request: the list is a compiled-in constant, it is small, and a second
// endpoint would be a second thing to authorise, rate-limit and document.
type catalogueEntry struct {
	ID      string
	Name    string
	Suggest shared.Suggestion
	Sources string // comma-separated, ready for the sources field
	Rows    []shared.ServicePort
}

// catalogueFor filters the catalogue to one protocol. A service with no port in
// this tab is left out: picking it would add nothing, and a button that does
// nothing is worse than an absent one.
func catalogueFor(proto string) []catalogueEntry {
	out := make([]catalogueEntry, 0, len(shared.Catalogue))
	for _, s := range shared.Catalogue {
		var rows []shared.ServicePort
		for _, p := range s.Ports {
			if p.Proto == proto {
				rows = append(rows, p)
			}
		}
		if len(rows) == 0 {
			continue
		}
		out = append(out, catalogueEntry{
			ID:      s.ID,
			Name:    s.Name,
			Suggest: s.Suggest,
			Sources: strings.Join(shared.SuggestedSources(s.Suggest), ", "),
			Rows:    rows,
		})
	}
	return out
}

func (s *Server) handlePortsGET(w http.ResponseWriter, r *http.Request) {
	ruleType := r.URL.Query().Get("type")
	if ruleType != "udp" {
		ruleType = "tcp"
	}

	state, err := s.client.GetRules()
	if err != nil {
		slog.Warn("get rules error", "error", err)
		s.render(w, r, "ports.html", "ports", &portsData{RuleType: ruleType, Catalogue: catalogueFor(ruleType)})
		return
	}

	rules := state.Staged.TCP
	if ruleType == "udp" {
		rules = state.Staged.UDP
	}
	s.render(w, r, "ports.html", "ports", &portsData{
		RuleType: ruleType, Rules: rules, Catalogue: catalogueFor(ruleType)})
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
		s.render(w, r, "ports.html", "ports", &portsData{
			RuleType: ruleType, Rules: rules, Catalogue: catalogueFor(ruleType)})
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
