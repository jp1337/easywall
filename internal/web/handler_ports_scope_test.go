package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The form posts a JSON array. A scope the browser sends must survive the round
// trip to the core rather than being dropped on the floor between them.
func TestHandlePortsPOST_TheScopeReachesTheCore(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	var saved []shared.PortRule
	fc.OnCommand(shared.CmdSaveRules, func(cmd shared.Command) {
		var p shared.SaveRulesPayload
		if err := json.Unmarshal(cmd.Payload, &p); err != nil {
			return
		}
		raw, _ := json.Marshal(p.Rules)
		_ = json.Unmarshal(raw, &saved)
	})

	rulesJSON := `[{"port":"25","description":"SMTP","ssh":false,"scope":"forwarded"}]`
	rec := doAuthFormRequest(t, s, "/ports", "type=tcp&rules="+urlEncode(rulesJSON))
	assertRedirect(t, rec, "/ports?type=tcp")

	if len(saved) != 1 {
		t.Fatalf("the core was asked to save %d rules, want 1: %+v", len(saved), saved)
	}
	if saved[0].Scope != shared.ScopeForwarded {
		t.Errorf("the scope did not survive the post: %+v", saved[0])
	}
}

// A scope nobody recognises is refused with the message that names it, not with
// "check core connection" — the reasoning handlePortsPOST's own validation
// comment already gives about incomplete rules.
func TestHandlePortsPOST_RefusesAnUnknownScope(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)

	var reached bool
	fc.OnCommand(shared.CmdSaveRules, func(shared.Command) { reached = true })

	rulesJSON := `[{"port":"25","description":"SMTP","scope":"sideways"}]`
	rec := doAuthFormRequest(t, s, "/ports", "type=tcp&rules="+urlEncode(rulesJSON))

	if reached {
		t.Error("a rule with an unknown scope was sent to the core")
	}
	assertStatus(t, rec, http.StatusOK) // re-rendered, not redirected
	body := rec.Body.String()
	if !strings.Contains(body, "save_invalid_ports") && !strings.Contains(body, "Nothing was saved") {
		t.Errorf("the operator was not told which rule was refused: %s", body)
	}
	if !strings.Contains(body, "SMTP") {
		t.Error("the row the operator was typing is not on the page that was re-rendered")
	}
}

// A forwarded rule does nothing at all while published_ports is "open": the
// forward chain takes no verdicts, so the rule is written, saved, applied — and
// never consulted. The page has to say so, which is the whole subject of 2.19.
//
// The three cases are one branch each in handlePortsGET: the note appears only
// when a rule on the page is forwarded *and* the core says published ports are
// open, and the core is not asked at all when no rule on the page cares.
func TestPortsGET_AForwardedRuleThatDoesNothingSaysSo(t *testing.T) {
	const noteID = `id="scope-inert"`

	cases := []struct {
		name           string
		publishedPorts string
		rules          []shared.PortRule
		wantNote       bool
		wantAsksCore   bool
	}{
		{
			name:           "open, and a rule is forwarded",
			publishedPorts: shared.PublishedPortsOpen,
			rules:          []shared.PortRule{{Port: "9000", Description: "MinIO", Scope: shared.ScopeForwarded}},
			wantNote:       true,
			wantAsksCore:   true,
		},
		{
			name:           "filtered, so the rule does what it says",
			publishedPorts: shared.PublishedPortsFiltered,
			rules:          []shared.PortRule{{Port: "9000", Description: "MinIO", Scope: shared.ScopeForwarded}},
			wantNote:       false,
			wantAsksCore:   true,
		},
		{
			name:           "open, but nothing on the page is forwarded",
			publishedPorts: shared.PublishedPortsOpen,
			rules:          []shared.PortRule{{Port: "443", Description: "HTTPS"}},
			wantNote:       false,
			wantAsksCore:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fc := newFakeCore(t)
			s := newTestServer(t, fc)
			enrollFactor(t, s)
			fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
				Staged: shared.Rules{TCP: tc.rules},
			}))
			fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{
				Docker: shared.DockerConfig{PublishedPorts: tc.publishedPorts},
			}))
			var asked int32
			fc.OnCommand(shared.CmdGetSettings, func(shared.Command) { atomic.AddInt32(&asked, 1) })

			rec := doAuthRequest(t, s, "GET", "/ports?type=tcp", nil)
			assertStatus(t, rec, http.StatusOK)

			if got := strings.Contains(rec.Body.String(), noteID); got != tc.wantNote {
				t.Errorf("the note about inert forwarded rules: on page = %v, want %v", got, tc.wantNote)
			}
			if got := atomic.LoadInt32(&asked) > 0; got != tc.wantAsksCore {
				t.Errorf("asked the core for [docker] = %v, want %v", got, tc.wantAsksCore)
			}
		})
	}
}

// The select is the only way an operator can set a scope, so it has to be in
// the table the page renders — with every value the core accepts, or a rule the
// config already carries cannot be edited without losing it.
func TestPortsGET_RendersTheScopeSelect(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Staged: shared.Rules{TCP: []shared.PortRule{
			{Port: "9000", Description: "MinIO", Scope: shared.ScopeBoth},
		}},
	}))

	rec := doAuthRequest(t, s, "GET", "/ports?type=tcp", nil)
	assertStatus(t, rec, http.StatusOK)
	body := rec.Body.String()

	if !strings.Contains(body, `class="f-scope`) {
		t.Fatal("the rule row carries no scope control; the column cannot be edited")
	}
	for _, v := range []shared.PortScope{shared.ScopeHost, shared.ScopeForwarded, shared.ScopeBoth} {
		if !strings.Contains(body, `value="`+string(v)+`"`) {
			t.Errorf("no option for scope %q — a rule set to it cannot be kept", v)
		}
	}
	// The stored value has to come back selected, or opening the page and
	// pressing Save silently rewrites every forwarded rule to host.
	if !strings.Contains(body, `value="both" selected`) {
		t.Error("the rule's own scope is not the selected option")
	}
}

// syncHidden rebuilds the whole rule list out of the DOM on every save, so a
// field the row does not read back is a field the save drops — the reasoning
// TestThePortsFormCarriesTheRuleIDBothWays spells out for the rule id. The
// scope is worse than the id: dropping it silently moves a container's rule
// back onto the input chain, where the packet it was written for never arrives.
func TestThePortsFormCarriesTheScopeBothWays(t *testing.T) {
	js := appJSSource(t)

	if !strings.Contains(js, ".f-scope") {
		t.Error("app.js never reads .f-scope; the scope the page rendered is dropped at the next save")
	}
	if !strings.Contains(js, "rule.scope = scope") {
		t.Error("app.js reads the scope but does not put it on the rule it submits")
	}
	if !strings.Contains(js, `scope !== 'host'`) {
		t.Error("app.js sends the scope unconditionally; host is the default and is " +
			"omitempty on the other side, so every rule in the file would gain a scope line")
	}
	// The row app.js builds has to be the row the server renders, or a rule
	// added in the browser has no scope control at all.
	if !strings.Contains(js, `class="f-scope`) {
		t.Error("the row app.js builds carries no scope control")
	}
}
