package web

import (
	"net/http"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHandleAllowlistGET_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/allowlist", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleAllowlistGET_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Staged: shared.Rules{Allowlist: []string{"10.0.0.1", "10.0.0.2/24"}},
	}))

	rec := doAuthRequest(t, s, "GET", "/allowlist", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleAllowlistGET_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetRules, errorRespFor("error"))

	rec := doAuthRequest(t, s, "GET", "/allowlist", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleAllowlistPOST_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/allowlist", "entries=10.0.0.1")
	assertRedirect(t, rec, "/login")
}

func TestHandleAllowlistPOST_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/allowlist", "entries=10.0.0.1%0A10.0.0.2")
	assertRedirect(t, rec, "/allowlist")
}

func TestHandleAllowlistPOST_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdSaveRules, errorRespFor("save failed"))

	rec := doAuthFormRequest(t, s, "/allowlist", "entries=10.0.0.1")
	assertRedirect(t, rec, "/allowlist")
}
