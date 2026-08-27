package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

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
