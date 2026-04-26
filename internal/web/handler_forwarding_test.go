package web

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHandleForwardingGET_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/forwarding", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleForwardingGET_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Staged: shared.Rules{
			Forwarding: []shared.ForwardingRule{
				{Protocol: "tcp", SourcePort: 8080, DestPort: 80},
			},
		},
	}))

	rec := doAuthRequest(t, s, "GET", "/forwarding", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleForwardingGET_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetRules, errorRespFor("rules error"))

	rec := doAuthRequest(t, s, "GET", "/forwarding", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleForwardingPOST_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/forwarding", "rules=[]")
	assertRedirect(t, rec, "/login")
}

func TestHandleForwardingPOST_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	rules := []shared.ForwardingRule{{Protocol: "tcp", SourcePort: 8080, DestPort: 80}}
	rulesJSON, _ := json.Marshal(rules)
	body := "rules=" + string(rulesJSON)

	rec := doAuthFormRequest(t, s, "/forwarding", body)
	assertRedirect(t, rec, "/forwarding")
}

func TestHandleForwardingPOST_InvalidJSON(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doAuthFormRequest(t, s, "/forwarding", "rules=not-valid-json")
	assertRedirect(t, rec, "/forwarding")
}

func TestHandleForwardingPOST_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveRules, errorRespFor("save error"))

	rec := doAuthFormRequest(t, s, "/forwarding", "rules=[]")
	assertRedirect(t, rec, "/forwarding")
}
