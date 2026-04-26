package web

import (
	"encoding/json"
	"net/http"
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
