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

	// Usage is what each rule has carried, keyed by rule id, and UsageKnown is
	// whether the core answered at all. The two are separate because they are
	// different claims: an empty map from a core that answered means no rule has
	// carried anything, and an empty map from one that did not means nothing is
	// known — the column renders "never" for the first and an em dash for the
	// second.
	Usage      map[string]shared.RuleUsage
	UsageKnown bool

	// ForwardedInert is true when a rule on this page is written for the
	// forward chain and the core is not taking verdicts there — published_ports
	// is "open", so Docker decides who reaches a published port and the rule is
	// never consulted. A rule that does nothing has to say so.
	ForwardedInert bool
}

// forwardedInert answers whether any rule on the page is waiting on a key that
// is switched off.
//
// The core is asked only when the answer could change what is rendered: on the
// overwhelming majority of hosts no rule is forwarded, and GET_SETTINGS would
// be a round trip whose result the page could not use. An unreadable answer
// says nothing rather than "your rule is inert" — the usage counters take the
// same line, and a warning nobody can act on is worse than no warning.
func (s *Server) forwardedInert(rules []shared.PortRule) bool {
	forwarded := false
	for _, r := range rules {
		if r.FiltersForwarded() {
			forwarded = true
			break
		}
	}
	if !forwarded {
		return false
	}
	nets, err := s.client.GetSettings()
	if err != nil {
		slog.Debug("could not read the docker settings for the ports page", "error", err)
		return false
	}
	return !nets.Docker.FiltersPublishedPorts()
}

// portUsage asks the core for the counters. A failure is logged and reported as
// "not known": the ports page is the rule editor first, and it has to work when
// the counters cannot be read.
func (s *Server) portUsage() (map[string]shared.RuleUsage, bool) {
	res, err := s.client.GetUsage()
	if err != nil {
		slog.Debug("could not read the port usage counters", "error", err)
		return nil, false
	}
	return res.Usage, true
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
		usage, usageKnown := s.portUsage()
		s.render(w, r, "ports.html", "ports", &portsData{
			RuleType: ruleType, Catalogue: catalogueFor(ruleType),
			Usage: usage, UsageKnown: usageKnown})
		return
	}

	rules := state.Staged.TCP
	if ruleType == "udp" {
		rules = state.Staged.UDP
	}
	usage, usageKnown := s.portUsage()
	s.render(w, r, "ports.html", "ports", &portsData{
		RuleType: ruleType, Rules: rules, Catalogue: catalogueFor(ruleType),
		Usage: usage, UsageKnown: usageKnown,
		ForwardedInert: s.forwardedInert(rules)})
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
		usage, usageKnown := s.portUsage()
		s.render(w, r, "ports.html", "ports", &portsData{
			RuleType: ruleType, Rules: rules, Catalogue: catalogueFor(ruleType),
			Usage: usage, UsageKnown: usageKnown,
			ForwardedInert: s.forwardedInert(rules)})
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
