package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHandlePortsGET_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/ports", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandlePortsGET_TCP(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Staged: shared.Rules{
			TCP: []shared.PortRule{{Port: "80", Description: "HTTP"}},
			UDP: []shared.PortRule{},
		},
	}))

	rec := doAuthRequest(t, s, "GET", "/ports?type=tcp", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlePortsGET_UDP(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Staged: shared.Rules{
			TCP: []shared.PortRule{},
			UDP: []shared.PortRule{{Port: "53", Description: "DNS"}},
		},
	}))

	rec := doAuthRequest(t, s, "GET", "/ports?type=udp", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlePortsGET_DefaultsToTCP(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{}))

	rec := doAuthRequest(t, s, "GET", "/ports", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlePortsGET_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetRules, errorRespFor("rules error"))

	rec := doAuthRequest(t, s, "GET", "/ports", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlePortsPOST_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/ports", "type=tcp&rules=[]")
	assertRedirect(t, rec, "/login")
}

func TestHandlePortsPOST_SavesTCPRules(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	rules := []shared.PortRule{{Port: "443", Description: "HTTPS"}}
	rulesJSON, _ := json.Marshal(rules)
	body := "type=tcp&rules=" + string(rulesJSON)

	rec := doAuthFormRequest(t, s, "/ports", body)
	assertRedirect(t, rec, "/ports?type=tcp")
}

func TestHandlePortsPOST_SavesUDPRules(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	rules := []shared.PortRule{{Port: "53", Description: "DNS"}}
	rulesJSON, _ := json.Marshal(rules)
	body := "type=udp&rules=" + string(rulesJSON)

	rec := doAuthFormRequest(t, s, "/ports", body)
	assertRedirect(t, rec, "/ports?type=udp")
}

func TestHandlePortsPOST_InvalidRulesJSON(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doAuthFormRequest(t, s, "/ports", "type=tcp&rules=not-json")
	assertRedirect(t, rec, "/ports?type=tcp")
}

func TestHandlePortsPOST_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveRules, errorRespFor("save error"))

	rec := doAuthFormRequest(t, s, "/ports", "type=tcp&rules=[]")
	assertRedirect(t, rec, "/ports?type=tcp")
}

func TestHandlePortsPOST_DefaultsToTCPType(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/ports", "rules=[]")
	assertRedirect(t, rec, "/ports?type=tcp")
}

// A row the operator started typing must not disappear.
//
// The editor used to drop any row without a port before submitting, so adding a
// row, writing the description first and pressing Save discarded the text with
// no message: the counter said nine rules, the table showed nine, and eight were
// sent. The browser now sends what is on screen and the answer comes from here —
// nothing is saved, the reason says what is missing, and the rows come back so
// the operator can finish the one they started.
func TestHandlePortsPOST_IncompleteRuleIsRejectedAndKeptOnScreen(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	var reached bool
	fc.OnCommand(shared.CmdSaveRules, func(shared.Command) { reached = true })

	rules := []shared.PortRule{
		{Port: "22", Description: "SSH"},
		{Port: "", Description: "Grafana, port to follow"},
	}
	payload, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}

	rec := doAuthFormRequest(t, s, "/ports", "type=tcp&rules="+urlEncode(string(payload)))

	if reached {
		t.Error("an incomplete rule set was forwarded to the core")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected the page to be re-rendered with the rejected rows, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Grafana, port to follow") {
		t.Error("the row the operator was typing is not in the response; " +
			"the message promises the rows are still on screen")
	}
	if !strings.Contains(body, "save_invalid_ports") && !strings.Contains(body, "Nothing was saved") {
		t.Error("the response does not say why nothing was saved")
	}
}

func TestHandlePortsPOST_CompleteRulesStillSave(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	var saved []shared.PortRule
	fc.OnCommand(shared.CmdSaveRules, func(cmd shared.Command) {
		var p shared.SaveRulesPayload
		_ = json.Unmarshal(cmd.Payload, &p)
		raw, _ := json.Marshal(p.Rules)
		_ = json.Unmarshal(raw, &saved)
	})

	payload, _ := json.Marshal([]shared.PortRule{{Port: "9090", Description: "Prometheus"}})
	rec := doAuthFormRequest(t, s, "/ports", "type=tcp&rules="+urlEncode(string(payload)))

	assertRedirect(t, rec, "/ports?type=tcp")
	if len(saved) != 1 || saved[0].Port != "9090" {
		t.Errorf("expected the valid rule to reach the core, got %+v", saved)
	}
}

// The sources round-trip: what the form posts is what the core is asked to save.
func TestHandlePortsPOST_KeepsSources(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
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

	rules := []shared.PortRule{
		{Port: "8123", Description: "Home Assistant", Sources: []string{"10.0.0.0/8", "192.168.0.0/16"}},
		{Port: "443", Description: "HTTPS"},
	}
	rulesJSON, _ := json.Marshal(rules)

	rec := doAuthFormRequest(t, s, "/ports", "type=tcp&rules="+string(rulesJSON))
	assertRedirect(t, rec, "/ports?type=tcp")

	if len(saved) != 2 {
		t.Fatalf("the core was asked to save %d rules, want 2: %+v", len(saved), saved)
	}
	if got := saved[0].Sources; len(got) != 2 || got[0] != "10.0.0.0/8" || got[1] != "192.168.0.0/16" {
		t.Errorf("sources = %v, want [10.0.0.0/8 192.168.0.0/16]", got)
	}
	if got := saved[1].Sources; len(got) != 0 {
		t.Errorf("an unrestricted rule arrived with sources %v", got)
	}
}

// The picker is rendered by the server, so it needs no route and no fetch: the
// entries are on the page, filtered to the tab's protocol.
func TestHandlePortsGET_RendersTheCatalogueForTheTab(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{}))

	rec := doAuthRequest(t, s, "GET", "/ports?type=tcp", nil)
	assertStatus(t, rec, http.StatusOK)
	tcp := rec.Body.String()
	if !strings.Contains(tcp, "Home Assistant") {
		t.Error("the TCP tab does not offer Home Assistant, which listens on 8123/tcp")
	}
	// "10.0.0.0/8" alone proves nothing: it is also in the ports_sources_hint
	// locale string ("Anywhere — or 10.0.0.0/8, 192.168.1.5"), which base.html
	// inlines into window.easywallStrings on every page. The full joined private
	// range list, anchored to Home Assistant's own data-service attribute, is
	// what only the picker can produce.
	if !strings.Contains(tcp, `data-service="homeassistant"`+"\n                    "+
		`data-sources="10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7"`) {
		t.Error("the private suggestion is not rendered into Home Assistant's catalogue item")
	}

	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{}))
	rec = doAuthRequest(t, s, "GET", "/ports?type=udp", nil)
	assertStatus(t, rec, http.StatusOK)
	udp := rec.Body.String()
	if !strings.Contains(udp, "WireGuard") {
		t.Error("the UDP tab does not offer WireGuard, which listens on 51820/udp")
	}
	if strings.Contains(udp, "Home Assistant") {
		t.Error("the UDP tab offers a service with no UDP port; picking it would add nothing")
	}
}

// The id is part of the payload, not something the server re-derives. A POST
// that carries one must forward it unchanged — the handler validates and
// forwards, and a rule stripped of its id here is a counter history thrown away.
func TestPortsPOST_ForwardsTheRuleID(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
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

	rulesJSON := `[{"id":"deadbeefcafe","port":"443","description":"HTTPS","ssh":false}]`
	rec := doAuthFormRequest(t, s, "/ports", "type=tcp&rules="+urlEncode(rulesJSON))
	assertRedirect(t, rec, "/ports?type=tcp")

	if len(saved) != 1 {
		t.Fatalf("want one saved rule, got %d", len(saved))
	}
	if got := saved[0].ID; got != "deadbeefcafe" {
		t.Errorf("stored id = %q, want deadbeefcafe", got)
	}
}

// A source that is not an address is refused with the message that names it, on
// the page still holding the operator's typing — the shape the port field has.
func TestHandlePortsPOST_RejectsAnInvalidSource(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	var reached bool
	fc.OnCommand(shared.CmdSaveRules, func(shared.Command) { reached = true })

	rulesJSON, _ := json.Marshal([]shared.PortRule{{Port: "443", Sources: []string{"nas.local"}}})
	rec := doAuthFormRequest(t, s, "/ports", "type=tcp&rules="+string(rulesJSON))

	assertStatus(t, rec, http.StatusOK) // re-rendered, not redirected
	if reached {
		t.Error("an invalid source reached the core")
	}
	if !strings.Contains(rec.Body.String(), "nas.local") {
		t.Error("the rejected source is not on the page that was re-rendered")
	}
}

// The column, end to end: a rule the core has usage for shows a date, the one
// it does not shows "never".
func TestPortsGET_RendersTheLastUsedColumn(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Staged: shared.Rules{
			TCP: []shared.PortRule{
				{ID: "aaaaaaaaaaaa", Port: "443", Description: "HTTPS"},
				{ID: "bbbbbbbbbbbb", Port: "80", Description: "HTTP"},
			},
		},
	}))
	fc.SetResponse(shared.CmdGetUsage, successResp(shared.UsageResult{
		Usage: map[string]shared.RuleUsage{
			// "bbbbbbbbbbbb" carries no entry at all: the rule that has never
			// carried a packet, which is the finding this column exists for.
			"aaaaaaaaaaaa": {LastSeen: time.Now().Add(-3 * 24 * time.Hour)},
		},
	}))

	rec := doAuthRequest(t, s, "GET", "/ports?type=tcp", nil)
	assertStatus(t, rec, http.StatusOK)
	body := rec.Body.String()

	if !strings.Contains(body, "Last used") {
		t.Error("the ports page has no Last used column heading")
	}
	if !strings.Contains(body, "never") {
		t.Error("no row reads never; one rule carries no usage entry, and " +
			"never is the finding this column exists for")
	}
	if !strings.Contains(body, "days ago") {
		t.Error("no row reads a date; either the usage never reached the template or " +
			"every rule is being rendered as unknown")
	}
}

// A core that can list rules but cannot answer GET_USAGE must not turn every
// port into "never" — the two commands are independent, and nothing was
// measured just because nothing could be read. The page still renders — it is
// the rule editor, and it works without the counters — and every cell claims
// nothing.
func TestPortsGET_SurvivesACoreThatCannotAnswerGetUsage(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Staged: shared.Rules{
			TCP: []shared.PortRule{{ID: "cccccccccccc", Port: "22", Description: "SSH"}},
		},
	}))
	fc.SetResponse(shared.CmdGetUsage, errorRespFor("usage store unavailable"))

	rec := doAuthRequest(t, s, "GET", "/ports?type=tcp", nil)
	assertStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), ">never<") {
		t.Error("a core that could not answer GET_USAGE produced never; nothing was measured, so " +
			"nothing may be claimed")
	}
}
