package web

import (
	"net/http"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHandleCustomGET_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/custom", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleCustomGET_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Staged: shared.Rules{Custom: []string{"# drop all invalid"}},
	}))

	rec := doAuthRequest(t, s, "GET", "/custom", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleCustomGET_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetRules, errorRespFor("error"))

	rec := doAuthRequest(t, s, "GET", "/custom", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleCustomPOST_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/custom", "rules=")
	assertRedirect(t, rec, "/login")
}

func TestHandleCustomPOST_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/custom", "rules=%23+comment%0Adrop+rule")
	assertRedirect(t, rec, "/custom")
}

func TestHandleCustomPOST_EmptyRules(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/custom", "rules=")
	assertRedirect(t, rec, "/custom")
}

func TestHandleCustomPOST_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveRules, errorRespFor("save failed"))

	rec := doAuthFormRequest(t, s, "/custom", "rules=some+rule")
	assertRedirect(t, rec, "/custom")
}
