package web

import (
	"net/http"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHandleBlacklistGET_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/blacklist", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleBlacklistGET_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Staged: shared.Rules{Blacklist: []string{"192.168.1.1"}},
	}))

	rec := doAuthRequest(t, s, "GET", "/blacklist", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleBlacklistGET_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetRules, errorRespFor("rules error"))

	rec := doAuthRequest(t, s, "GET", "/blacklist", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleBlacklistPOST_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/blacklist", "entries=192.168.1.1")
	assertRedirect(t, rec, "/login")
}

func TestHandleBlacklistPOST_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/blacklist", "entries=192.168.1.1%0A10.0.0.1")
	assertRedirect(t, rec, "/blacklist")
}

func TestHandleBlacklistPOST_EmptyEntries(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/blacklist", "entries=")
	assertRedirect(t, rec, "/blacklist")
}

func TestHandleBlacklistPOST_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveRules, errorRespFor("save error"))

	rec := doAuthFormRequest(t, s, "/blacklist", "entries=192.168.1.1")
	assertRedirect(t, rec, "/blacklist")
}
