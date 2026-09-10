package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jp1337/easywall/internal/shared"
)

type systemData struct {
	Settings  *shared.SystemSettings
	CoreErr   string
	Telemetry bool

	// ACMEEnabled gates the port-80 status row: it means nothing to an
	// operator who is not using ACME, and the report below has nothing to say
	// when usesACME() is false.
	ACMEEnabled bool

	// Port80State is one of port80Open, port80Restricted or
	// port80NotCovered, carried as a plain string so the template's {{eq}}
	// compares against a string literal rather than a named type. Meaningless
	// when Port80Known is false.
	Port80State string
	Port80Known bool

	// TelemetryEndpoint is shown on the page. "A random identifier and the
	// version" is only a checkable claim if the operator can also see where it
	// goes, and being told that in the interface beats having to find the
	// documentation.
	TelemetryEndpoint string

	// TelemetryProv is the marker beside the consent toggle, or nil when
	// EASYWALL_WEB_TELEMETRY is unset — which is every installation that does
	// not set it, and why the template guards with {{with}}.
	TelemetryProv *provenanceView
}

func (s *Server) handleSystemGET(w http.ResponseWriter, r *http.Request) {
	data := &systemData{
		Telemetry:         s.cfg.TelemetryEnabled(),
		TelemetryEndpoint: shared.TelemetryEndpoint,
		TelemetryProv:     s.provenanceFor("telemetry"),
		ACMEEnabled:       s.certs.usesACME(),
	}
	if data.ACMEEnabled {
		state, known := s.port80Reachability()
		data.Port80State = string(state)
		data.Port80Known = known
	}

	settings, err := s.client.GetSystem()
	if err != nil {
		slog.Warn("could not get system settings", "error", err)
		data.CoreErr = err.Error()
	} else {
		data.Settings = settings
	}

	s.render(w, r, "system.html", "system", data)
}

func (s *Server) handleSystemPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// The number input carries min and max attributes, but those are a courtesy
	// to the browser: a form posted any other way ignores them. A one-second
	// window rolls back before the confirmation page can be read, which leaves
	// the firewall unchangeable through the interface.
	dur, err := strconv.Atoi(r.FormValue("acceptance_duration"))
	if err != nil || !shared.ValidAcceptanceDuration(dur) {
		s.respondPartialError(w, r, "/system", "system_invalid_duration")
		return
	}

	settings := shared.SystemSettings{
		Acceptance: shared.AcceptanceConfig{
			Enabled:  r.FormValue("acceptance_enabled") == "on",
			Duration: dur,
		},
	}

	if err := s.client.SaveSystem(settings); err != nil {
		slog.Warn("could not save system settings", "error", err)
		s.respondPartialError(w, r, "/system", "save_error")
		return
	}

	s.respondPartialSave(w, r, "/system", "system_saved")
}

// handleTelemetryPOST records the answer to being counted.
//
// Its own route, and deliberately not part of the settings save above: that one
// goes through the core, and consent that can only be withdrawn while another
// process is reachable is not consent. This writes web.toml and nothing else.
func (s *Server) handleTelemetryPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// The reset button is a second submit inside the same form, so it arrives
	// here rather than through a route of its own. It clears the stored answer;
	// the checkbox's value is irrelevant to it and is deliberately not read.
	if r.FormValue("reset") != "" {
		if err := s.cfg.ResetTelemetry(); err != nil {
			slog.Warn("could not reset the telemetry setting", "error", err)
			s.respondPartialError(w, r, "/system", "save_error")
			return
		}
		slog.Info("telemetry setting reset to the environment")

		if !isHTMX(r) {
			s.respondPartialSave(w, r, "/system", "provenance_reset_done")
			return
		}

		// respondPartialSave's usual 204 leaves htmx with nothing to swap, and
		// a reset changes what is true on screen, not just what is stored: the
		// environment's value is now in force, so the checkbox may no longer
		// match what it displayed a moment ago, and the button the operator
		// just clicked must disappear — there is nothing left for it to
		// reset. This release exists to stop the interface asserting a
		// provenance it does not have; leaving the just-superseded state on
		// screen after the very action that superseded it would be that same
		// mistake. hx-swap="none" only suppresses the main response target;
		// htmx still applies hx-swap-oob elements found in the body, which is
		// how the checkbox and marker get back in sync without a page load.
		w.Header().Set("HX-Trigger", `{"easywall:saved":"provenance_reset_done"}`)
		s.renderPartial(w, r, "telemetry_state_oob", &systemData{
			Telemetry:     s.cfg.TelemetryEnabled(),
			TelemetryProv: s.provenanceFor("telemetry"),
		})
		return
	}

	enabled := r.FormValue("telemetry") == "on"
	if err := s.cfg.SaveTelemetry(enabled); err != nil {
		slog.Warn("could not save the telemetry setting", "error", err)
		s.respondPartialError(w, r, "/system", "save_error")
		return
	}

	// The reporter reads consent through the config on every attempt, so there
	// is nothing to restart: switching off here means the next attempt sends
	// nothing.
	slog.Info("telemetry setting changed", "enabled", enabled)
	s.respondPartialSave(w, r, "/system", "system_saved")
}

// port80State is what easywall's own rules say about the port ACME needs —
// see port80Reachability for why "open" and "closed" are not enough.
type port80State string

const (
	// port80Open: a rule covers 80 — a single value or a range — and its
	// Sources is empty, so anything, including a certificate authority on the
	// public internet, may reach it.
	port80Open port80State = "open"

	// port80Restricted: a rule covers 80, but its Sources narrows who may use
	// it. easywall does not know the certificate authority's addresses — they
	// are not fixed, and easywall does not track them — so this is the honest
	// answer rather than a guess in either direction.
	port80Restricted port80State = "restricted"

	// port80NotCovered: no rule admits port 80. Also what a port value this
	// parser cannot read reports as — see portRuleCovers80.
	port80NotCovered port80State = "not_covered"
)

// port80Reachability reports what easywall's own rules say about the port
// ACME needs — never what easywall opened, because it opens nothing on port
// 80 by itself. A firewall program that opens ports on its own initiative is
// not one an operator can reason about; what it can do is look, and say.
//
// known is false when the core could not be asked at all. An unanswered
// question is reported as unanswered, never folded into "not covered": an
// operator whose core is down must not be told their firewall is blocking a
// port when nothing was actually asked. state is the zero value in that case
// and the caller must check known first.
//
// A rule "covers" 80 by single value or by range containment — 79:81 admits
// 80 exactly as a bare "80" does, and reporting a covering range as "not
// covered" would send the operator to add a rule they already have. Coverage
// alone is not the whole answer, though: PortRule.Sources can restrict even a
// covering rule to addresses that do not include the certificate authority.
// Every rule is checked for the open case — an empty Sources — before any
// rule is allowed to answer restricted, because one unrestricted covering
// rule makes the port open even if a second, narrower rule also covers it.
//
// Reads the *live* rule set (state.Current), not what is staged for the next
// apply: the question is whether a certificate authority can reach this host
// right now, and an unapplied change has not touched the kernel yet.
func (s *Server) port80Reachability() (state port80State, known bool) {
	rules, err := s.client.GetRules()
	if err != nil {
		return "", false
	}
	restricted := false
	for _, p := range rules.Current.TCP {
		if !portRuleCovers80(p.Port) {
			continue
		}
		if len(p.Sources) == 0 {
			return port80Open, true
		}
		restricted = true
	}
	if restricted {
		return port80Restricted, true
	}
	return port80NotCovered, true
}

// portRuleCovers80 reports whether a PortRule.Port value — a single port or a
// "low:high" range — admits port 80.
//
// A value this parser cannot read is treated as not covering it, never as an
// error or a panic: this is a status row, and a rule the core already
// accepted in some shape this parser does not recognise must not take the
// page down over it.
func portRuleCovers80(port string) bool {
	if port == "80" {
		return true
	}
	low, high, ok := strings.Cut(port, ":")
	if !ok {
		return false
	}
	lo, errLo := strconv.Atoi(low)
	hi, errHi := strconv.Atoi(high)
	if errLo != nil || errHi != nil {
		return false
	}
	return lo <= 80 && 80 <= hi
}
