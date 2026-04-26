package web

import (
	"net/http"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHandleWhitelistGET_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/whitelist", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleWhitelistGET_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Staged: shared.Rules{Whitelist: []string{"10.0.0.1", "10.0.0.2/24"}},
	}))

	rec := doAuthRequest(t, s, "GET", "/whitelist", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleWhitelistGET_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetRules, errorRespFor("error"))

	rec := doAuthRequest(t, s, "GET", "/whitelist", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleWhitelistPOST_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/whitelist", "entries=10.0.0.1")
	assertRedirect(t, rec, "/login")
}

func TestHandleWhitelistPOST_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/whitelist", "entries=10.0.0.1%0A10.0.0.2")
	assertRedirect(t, rec, "/whitelist")
}

func TestHandleWhitelistPOST_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveRules, errorRespFor("save failed"))

	rec := doAuthFormRequest(t, s, "/whitelist", "entries=10.0.0.1")
	assertRedirect(t, rec, "/whitelist")
}
