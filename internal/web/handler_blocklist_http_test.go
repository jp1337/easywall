package web

import (
	"net/http"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHandleBlocklistGET_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/blocklist", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleBlocklistGET_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Staged: shared.Rules{Blocklist: []string{"192.168.1.1"}},
	}))

	rec := doAuthRequest(t, s, "GET", "/blocklist", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleBlocklistGET_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetRules, errorRespFor("rules error"))

	rec := doAuthRequest(t, s, "GET", "/blocklist", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleBlocklistPOST_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/blocklist", "entries=192.168.1.1")
	assertRedirect(t, rec, "/login")
}

func TestHandleBlocklistPOST_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/blocklist", "entries=192.168.1.1%0A10.0.0.1")
	assertRedirect(t, rec, "/blocklist")
}

func TestHandleBlocklistPOST_EmptyEntries(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/blocklist", "entries=")
	assertRedirect(t, rec, "/blocklist")
}

func TestHandleBlocklistPOST_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdSaveRules, errorRespFor("save error"))

	rec := doAuthFormRequest(t, s, "/blocklist", "entries=192.168.1.1")
	assertRedirect(t, rec, "/blocklist")
}
