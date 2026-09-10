package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/nicksnyder/go-i18n/v2/i18n"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHandleSystemGET_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/system", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleSystemGET_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetSystem, successResp(shared.SystemSettings{
		Acceptance: shared.AcceptanceConfig{Enabled: true, Duration: 120},
	}))

	rec := doAuthRequest(t, s, "GET", "/system", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleSystemGET_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetSystem, errorRespFor("core unavailable"))

	rec := doAuthRequest(t, s, "GET", "/system", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleSystemPOST_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/system", "acceptance_duration=120")
	assertRedirect(t, rec, "/login")
}

func TestHandleSystemPOST_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdSaveSystem, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/system",
		"acceptance_enabled=on&acceptance_duration=60")
	assertRedirect(t, rec, "/system")
}

func TestHandleSystemPOST_InvalidDuration(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)

	rec := doAuthFormRequest(t, s, "/system", "acceptance_duration=abc")
	assertRedirect(t, rec, "/system")
}

// The number input advertises min=10 and max=3600. That is a browser hint, and
// the handler took any positive number — so a one-second window, which rolls
// back before the confirmation page can even be read, went through and made the
// firewall unchangeable through the interface.
func TestHandleSystemPOST_RejectsADurationOutsideTheAdvertisedRange(t *testing.T) {
	for _, dur := range []string{"0", "1", "9", "3601", "86400", "-5"} {
		t.Run(dur, func(t *testing.T) {
			fc := newFakeCore(t)
			s := newTestServer(t, fc)
			enrollFactor(t, s)
			fc.SetResponse(shared.CmdSaveSystem, shared.Response{Success: true})

			rec := doAuthFormRequest(t, s, "/system",
				"acceptance_enabled=on&acceptance_duration="+dur)
			assertRedirect(t, rec, "/system")

			if cmd := fc.LastCommand(); cmd != nil {
				t.Errorf("duration %s must not reach the core, got command %q", dur, cmd.Type)
			}
		})
	}
}

func TestHandleSystemPOST_AcceptsTheRangeBoundaries(t *testing.T) {
	for _, dur := range []string{"10", "3600"} {
		t.Run(dur, func(t *testing.T) {
			fc := newFakeCore(t)
			s := newTestServer(t, fc)
			enrollFactor(t, s)
			fc.SetResponse(shared.CmdSaveSystem, shared.Response{Success: true})

			rec := doAuthFormRequest(t, s, "/system",
				"acceptance_enabled=on&acceptance_duration="+dur)
			assertRedirect(t, rec, "/system")

			cmd := fc.LastCommand()
			if cmd == nil || cmd.Type != shared.CmdSaveSystem {
				t.Errorf("duration %s is permitted and must be saved, got %v", dur, cmd)
			}
		})
	}
}

func TestHandleSystemPOST_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdSaveSystem, errorRespFor("save failed"))

	rec := doAuthFormRequest(t, s, "/system", "acceptance_duration=120")
	assertRedirect(t, rec, "/system")
}

// ── HTMX path: respondPartialSave / respondPartialError ──────────────────────

// doAuthFormHTMX performs an authenticated POST with the HX-Request header
// set so the server takes the partial-save code path.
func doAuthFormHTMX(t *testing.T, s *Server, url, formBody string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", url, strings.NewReader(formBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(makeAuthCookie(t, s))
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

func TestHandleSystemPOST_HTMX_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)

	rec := doAuthFormHTMX(t, s, "/system", "acceptance_enabled=on&acceptance_duration=60")
	assertStatus(t, rec, http.StatusNoContent)
	trigger := rec.Header().Get("HX-Trigger")
	if !strings.Contains(trigger, "easywall:saved") {
		t.Errorf("expected HX-Trigger easywall:saved, got %q", trigger)
	}
	if !strings.Contains(trigger, "system_saved") {
		t.Errorf("expected flash key 'system_saved' in trigger, got %q", trigger)
	}
}

func TestHandleSystemPOST_HTMX_InvalidDuration(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)

	rec := doAuthFormHTMX(t, s, "/system", "acceptance_duration=0")
	assertStatus(t, rec, http.StatusOK)
	trigger := rec.Header().Get("HX-Trigger")
	if !strings.Contains(trigger, "easywall:error") {
		t.Errorf("expected HX-Trigger easywall:error, got %q", trigger)
	}
	if !strings.Contains(trigger, "system_invalid_duration") {
		t.Errorf("expected flash key 'system_invalid_duration' in trigger, got %q", trigger)
	}
}

func TestHandleSystemPOST_HTMX_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdSaveSystem, errorRespFor("save failed"))

	rec := doAuthFormHTMX(t, s, "/system", "acceptance_duration=120")
	assertStatus(t, rec, http.StatusOK)
	trigger := rec.Header().Get("HX-Trigger")
	if !strings.Contains(trigger, "easywall:error") {
		t.Errorf("expected HX-Trigger easywall:error, got %q", trigger)
	}
}

// ── Telemetry ───────────────────────────────────────────────────────────────

// Withdrawing consent goes through its own route on purpose: the settings on
// the same page travel through the core, and a core that cannot be reached
// must not be able to keep an installation counted.
func TestHandleTelemetryPOST_WorksWithoutTheCore(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	if err := s.cfg.SaveTelemetry(true); err != nil {
		t.Fatal(err)
	}
	fc.listener.Close() // the core is gone from here on

	rec := doFormRequest(s, "POST", "/system/telemetry", "", makeAuthCookie(t, s))
	if rec.Code >= 400 {
		t.Fatalf("consent could not be withdrawn: HTTP %d", rec.Code)
	}
	if s.cfg.TelemetryEnabled() {
		t.Error("the installation is still counted after the switch was turned off")
	}
}

func TestHandleTelemetryPOST_RecordsConsent(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)

	doFormRequest(s, "POST", "/system/telemetry", "telemetry=on", makeAuthCookie(t, s))
	if !s.cfg.TelemetryEnabled() {
		t.Error("switching it on was not recorded")
	}

	doFormRequest(s, "POST", "/system/telemetry", "", makeAuthCookie(t, s))
	if s.cfg.TelemetryEnabled() {
		t.Error("switching it off was not recorded")
	}
}

// The reset button is a second submit inside the telemetry form (name="reset"),
// and it must take a different path through handleTelemetryPOST than an
// ordinary save: it removes the stored line rather than writing one. A
// mutation that disabled the "is this a reset" check would fold a bare
// reset=1 submission (no telemetry checkbox in the body at all) into an
// ordinary save of "off" — a save wearing a reset button, and one that would
// have quietly reintroduced the exact "changed here, reverted after a
// restart" bug 2.12 exists to remove.
func TestHandleTelemetryPOST_ResetRemovesTheStoredLine(t *testing.T) {
	t.Setenv("EASYWALL_WEB_TELEMETRY", "true")
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	if err := s.cfg.SaveTelemetry(false); err != nil {
		t.Fatalf("SaveTelemetry: %v", err)
	}
	if s.cfg.TelemetryEnabled() {
		t.Fatal("the stored no did not win, so this test starts from the wrong state")
	}

	rec := doFormRequest(s, "POST", "/system/telemetry", "reset=1", makeAuthCookie(t, s))
	if rec.Code >= 400 {
		t.Fatalf("the reset request failed: HTTP %d", rec.Code)
	}

	written, err := os.ReadFile(s.cfg.configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(written), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "telemetry") {
			t.Errorf("the stored line survived the reset: %q", trimmed)
		}
	}
	if !s.cfg.TelemetryEnabled() {
		t.Error("after the reset the environment's true is not in force")
	}
}

// submitButtonRe matches one <button type="submit" ...> opening tag, used to
// find the first submit button in a form in tree order.
var submitButtonRe = regexp.MustCompile(`(?s)<button[^>]*type="submit"[^>]*>`)

// The reset button rendered by the generic "provenance" block sits ahead of
// the Save button in the telemetry form's source. HTML defines a form's
// default button — the one an Enter key press activates — as simply the
// first submit button in tree order, with no exception for one the browser
// does not render. Focus the telemetry toggle, press Enter to "save", and
// without a guard ahead of it, the browser activates the reset button
// instead: exactly the state where a stored answer is beating the
// environment, an operator trying to save their choice instead deletes it
// and hands consent back to the environment.
func TestHandleSystemGET_TelemetryResetIsNeverTheDefaultButton(t *testing.T) {
	t.Setenv("EASYWALL_WEB_TELEMETRY", "true")
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	if err := s.cfg.SaveTelemetry(false); err != nil {
		t.Fatalf("SaveTelemetry: %v", err)
	}

	rec := doRequest(s, "GET", "/system", nil, makeAuthCookie(t, s))
	body := rec.Body.String()
	if !strings.Contains(body, `name="reset"`) {
		t.Fatal("the stored value does not conflict with the environment here, " +
			"so this test is not exercising the reset button at all")
	}

	var telemetryForm string
	for _, form := range postForms(body) {
		if strings.Contains(form, `action="/system/telemetry"`) {
			telemetryForm = form
			break
		}
	}
	if telemetryForm == "" {
		t.Fatal("could not find the telemetry form on the rendered page")
	}

	first := submitButtonRe.FindString(telemetryForm)
	if first == "" {
		t.Fatal("the telemetry form has no submit button at all")
	}
	if strings.Contains(first, `name="reset"`) {
		t.Errorf("the reset button is the telemetry form's default button:\n%s\n"+
			"  focusing the toggle and pressing Enter resets instead of saving", first)
	}
}

// Over HTMX, hx-swap="none" means the main response body is normally never
// applied to the page — the toast is the only feedback. A reset is the one
// action on this route that changes what is *shown*, not just what is
// stored: the checkbox may no longer match its own on-screen state, and the
// reset button just clicked must disappear now there is nothing left to
// reset. Left alone, the operator would see their own superseded answer for
// as long as they stay on the page — the exact "the interface asserts a
// provenance it does not have" defect this release exists to remove.
func TestHandleTelemetryPOST_HTMX_ResetUpdatesTheDOM(t *testing.T) {
	t.Setenv("EASYWALL_WEB_TELEMETRY", "true")
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	if err := s.cfg.SaveTelemetry(false); err != nil {
		t.Fatalf("SaveTelemetry: %v", err)
	}

	rec := doAuthFormHTMX(t, s, "/system/telemetry", "reset=1")
	assertStatus(t, rec, http.StatusOK)

	body := rec.Body.String()
	if !strings.Contains(body, `hx-swap-oob="true"`) {
		t.Fatalf("the response carries no out-of-band swap, so the stale checkbox "+
			"and reset button are left on screen: %q", body)
	}
	if strings.Contains(body, `name="reset"`) {
		t.Error("the reset button is still in the response after a successful reset; " +
			"there is nothing left for it to reset")
	}
	if !strings.Contains(body, "checked") {
		t.Error("the returned checkbox does not reflect the environment's true value")
	}

	trigger := rec.Header().Get("HX-Trigger")
	if !strings.Contains(trigger, "easywall:saved") || !strings.Contains(trigger, "provenance_reset_done") {
		t.Errorf("expected the usual save toast to still fire, got %q", trigger)
	}
}

// The public demo is wiped every few hours, identifier included. Left counting,
// it would invent several installations a day in a number whose entire value is
// being small enough to mean something.
func TestNewServer_DemoModeNeverCounts(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	s.cfg.DemoMode = true

	demo, err := NewServer(s.cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer demo.Stop()

	if demo.telemetry != nil {
		t.Error("the demo would report itself as an installation")
	}
}

// The System page has to show what is sent and where — a claim about outbound
// traffic that the interface itself does not state is one an operator has to
// take on faith.
func TestHandleSystemGET_NamesTheTelemetryEndpoint(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)

	rec := doRequest(s, "GET", "/system", nil, makeAuthCookie(t, s))
	body := rec.Body.String()
	if !strings.Contains(body, shared.TelemetryEndpoint) {
		t.Errorf("the System page does not say where reports go (%q)", shared.TelemetryEndpoint)
	}
	if !strings.Contains(body, `action="/system/telemetry"`) {
		t.Error("the telemetry switch is not on its own form; a core outage would block it")
	}
}

// TestTheSystemPageHasNoTwoButtonsWithOneName asserts that no visible button
// label appears twice on /system.
//
// It rendered two buttons reading "Save system settings", one after the
// acceptance window and one after the installation count, and nothing said
// which section each belonged to. Written over the rendered page rather than
// over the template so that a label moved into a partial is still caught.
func TestTheSystemPageHasNoTwoButtonsWithOneName(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetSystem, successResp(shared.SystemSettings{
		Acceptance: shared.AcceptanceConfig{Enabled: true, Duration: 120},
	}))

	rec := doRequest(s, "GET", "/system", nil, makeAuthCookie(t, s))
	body := rec.Body.String()

	labels := buttonLabels(body)
	seen := map[string]int{}
	for _, l := range labels {
		seen[l]++
	}
	for label, n := range seen {
		if n > 1 {
			t.Errorf("the label %q appears on %d buttons; a reader cannot tell which section each saves", label, n)
		}
	}
	if len(labels) < 2 {
		t.Fatalf("expected at least two buttons on /system, found %d — the test is no longer looking at the right page", len(labels))
	}
}

// buttonLabels returns the visible text of every <button> in html.
func buttonLabels(html string) []string {
	var out []string
	re := regexp.MustCompile(`(?s)<button[^>]*>(.*?)</button>`)
	tags := regexp.MustCompile(`<[^>]*>`)
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		text := strings.TrimSpace(tags.ReplaceAllString(m[1], " "))
		text = strings.Join(strings.Fields(text), " ")
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

// ── ACME port-80 report ───────────────────────────────────────────────────

// newACMESystemTestServer builds a Server the way newTestServer does, then
// turns tls.acme on and attaches a real certManager for it: the four-state
// port-80 report has nothing to say while usesACME() is false, and only a
// certManager built from an ACME-on config answers true.
func newACMESystemTestServer(t *testing.T, opts ...func(*fakeCore)) *Server {
	t.Helper()
	fc := newFakeCore(t)
	for _, opt := range opts {
		opt(fc)
	}
	s := newTestServer(t, fc)
	enrollFactor(t, s)

	s.cfg.TLS.ACME = true
	s.cfg.TLS.Hostname = "firewall.example.org"
	s.cfg.TLS.ACMEAgreeTOS = true
	certs, err := newCertManager(s.cfg)
	if err != nil {
		t.Fatalf("newCertManager: %v", err)
	}
	s.certs = certs
	return s
}

// withPortRules configures the fake core's GetRules response with the given
// rules as the *live* TCP set (RulesState.Current) — port80Reachability reads
// Current, not Staged, because the question is whether a certificate
// authority can reach the host right now.
func withPortRules(rules ...shared.PortRule) func(*fakeCore) {
	return func(fc *fakeCore) {
		fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
			Current: shared.Rules{TCP: rules},
		}))
	}
}

// withTCPPorts is withPortRules for the common case: one rule per port, no
// Sources restriction.
func withTCPPorts(ports ...string) func(*fakeCore) {
	rules := make([]shared.PortRule, len(ports))
	for i, p := range ports {
		rules[i] = shared.PortRule{Port: p}
	}
	return withPortRules(rules...)
}

// withCoreUnreachable closes the fake core's listener before the server ever
// gets a chance to dial it — the same shape TestHandleTelemetryPOST_WorksWithoutTheCore
// uses, so a request meets a real dial failure rather than a canned error.
func withCoreUnreachable() func(*fakeCore) {
	return func(fc *fakeCore) { fc.listener.Close() }
}

// getAuthedBody performs an authenticated GET and returns the response body,
// for tests that only care about what rendered.
func (s *Server) getAuthedBody(t *testing.T, path string) string {
	t.Helper()
	return doAuthRequest(t, s, http.MethodGet, path, nil).Body.String()
}

// translated returns the English translation of a message id, using the
// package's shared test bundle — so a test can assert against the rendered
// sentence rather than the raw id, the same text an operator actually reads.
func translated(t *testing.T, id string) string {
	t.Helper()
	loc := i18n.NewLocalizer(testBundle(t), "en")
	return T(loc, id)
}

// stagedTCP re-reads the rules and returns the staged TCP set — a test helper
// so a test can assert that answering the port-80 question never turned into
// a write that staged the port itself.
func (c *CoreClient) stagedTCP() []shared.PortRule {
	state, err := c.GetRules()
	if err != nil {
		return nil
	}
	return state.Staged.TCP
}

// containsPort reports whether any rule in rules opens exactly port.
func containsPort(rules []shared.PortRule, port string) bool {
	for _, r := range rules {
		if r.Port == port {
			return true
		}
	}
	return false
}

// TestTheSystemPageReportsWhetherPortEightyIsOpen asserts easywall measures
// rather than asserts, and does not open the port itself.
//
// ACME needs port 80 reachable. easywall is the firewall in front of it, so
// the operator has to open it — and a firewall program that opens ports on
// its own initiative contradicts the whole design. What it can do is look and
// say.
//
// Four states, because that is what the truth has: a rule (single value or
// range) may or may not cover port 80, and a covering rule's Sources may or
// may not restrict who reaches it. Matching "80" alone would call a rule of
// 79:81 "not covered" and send an operator to add a rule they already have;
// ignoring Sources would call a rule restricted to 10.0.0.0/8 "open" to a
// certificate authority that reaches them from the public internet. Neither
// is the honest answer.
func TestTheSystemPageReportsWhetherPortEightyIsOpen(t *testing.T) {
	t.Run("closed", func(t *testing.T) {
		s := newACMESystemTestServer(t, withTCPPorts("22", "12227"))
		body := s.getAuthedBody(t, "/system")
		if !strings.Contains(body, "acme_port_closed") && !strings.Contains(body, translated(t, "acme_port_closed")) {
			t.Error("port 80 is not in the rule set and the page did not say so")
		}
		// And it did not stage it.
		if containsPort(s.client.stagedTCP(), "80") {
			t.Fatal("easywall staged port 80 by itself")
		}
	})

	t.Run("open", func(t *testing.T) {
		s := newACMESystemTestServer(t, withTCPPorts("22", "80", "12227"))
		body := s.getAuthedBody(t, "/system")
		if strings.Contains(body, translated(t, "acme_port_closed")) {
			t.Error("port 80 is in the rule set and the page said it was closed")
		}
		if !strings.Contains(body, translated(t, "acme_port_open")) {
			t.Error("an unrestricted rule for 80 did not render as open")
		}
	})

	// A range containing 80 admits it exactly as a bare "80" does. Reporting
	// this "not covered" would send the operator to add a rule they already
	// have — the mistake the plan this task replaced would have made.
	t.Run("open via range", func(t *testing.T) {
		s := newACMESystemTestServer(t, withPortRules(shared.PortRule{Port: "79:81"}))
		body := s.getAuthedBody(t, "/system")
		if !strings.Contains(body, translated(t, "acme_port_open")) {
			t.Error("a range covering 80 did not render as open")
		}
	})

	// A range that does not reach 80 must not be confused with one that does.
	t.Run("range does not cover 80", func(t *testing.T) {
		s := newACMESystemTestServer(t, withPortRules(shared.PortRule{Port: "8000:9000"}))
		body := s.getAuthedBody(t, "/system")
		if !strings.Contains(body, translated(t, "acme_port_closed")) {
			t.Error("a range that does not reach 80 did not render as not covered")
		}
	})

	// A rule for 80 restricted to specific sources does not let a certificate
	// authority on the public internet in. Calling that "open" is the false
	// reassurance in the other direction — the second thing the plan this
	// task replaced would have missed entirely.
	t.Run("restricted", func(t *testing.T) {
		s := newACMESystemTestServer(t, withPortRules(
			shared.PortRule{Port: "80", Sources: []string{"10.0.0.0/8"}}))
		body := s.getAuthedBody(t, "/system")
		if strings.Contains(body, translated(t, "acme_port_open")) {
			t.Error("a rule restricted to a private network rendered as open to anyone")
		}
		if strings.Contains(body, translated(t, "acme_port_closed")) {
			t.Error("a rule that does cover 80 rendered as not covered at all")
		}
		if !strings.Contains(body, translated(t, "acme_port_restricted")) {
			t.Error("a restricted rule for 80 did not say so")
		}
	})

	t.Run("no rules at all", func(t *testing.T) {
		s := newACMESystemTestServer(t, withTCPPorts())
		body := s.getAuthedBody(t, "/system")
		if !strings.Contains(body, translated(t, "acme_port_closed")) {
			t.Error("an empty rule set did not render as not covered")
		}
	})

	// A port value this parser cannot read must not take the page down — this
	// is a status row about a rule the core already accepted in some shape.
	// Two shapes: no colon at all, and a colon with a non-numeric half — the
	// second is the one that reaches the range parser's own error path rather
	// than being turned away before it.
	for _, malformed := range []string{"not-a-port", "abc:def"} {
		t.Run("malformed port value/"+malformed, func(t *testing.T) {
			s := newACMESystemTestServer(t, withPortRules(shared.PortRule{Port: malformed}))
			body := s.getAuthedBody(t, "/system")
			if !strings.Contains(body, translated(t, "acme_port_closed")) {
				t.Errorf("port value %q did not render as not covered", malformed)
			}
		})
	}

	// The core cannot be asked at all. Reported as unknown, never folded into
	// "not covered" — an operator whose core is down must not be told their
	// firewall is blocking a port when nothing was actually asked.
	t.Run("unknown", func(t *testing.T) {
		s := newACMESystemTestServer(t, withCoreUnreachable())
		body := s.getAuthedBody(t, "/system")
		if strings.Contains(body, translated(t, "acme_port_closed")) {
			t.Error("an unreachable core rendered as not covered instead of unknown")
		}
		if strings.Contains(body, translated(t, "acme_port_open")) {
			t.Error("an unreachable core rendered as open")
		}
		if !strings.Contains(body, translated(t, "acme_port_unknown")) {
			t.Error("an unreachable core did not render as unknown")
		}
	})
}

// TestTheSystemPageHasNoPortEightyReportWithoutACME asserts the row is
// invisible on every installation that has not turned ACME on — which is
// most of them — rather than showing a report about a feature that is off.
func TestTheSystemPageHasNoPortEightyReportWithoutACME(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)

	body := s.getAuthedBody(t, "/system")
	if strings.Contains(body, translated(t, "acme_port_label")) {
		t.Error("the port-80 report is shown even though ACME is off")
	}
}
