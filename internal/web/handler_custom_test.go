package web

import (
	"encoding/json"
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

// TestHandleCustomPOST_Valid exercises the happy path: validation passes, save
// succeeds, and the handler redirects to /custom.
func TestHandleCustomPOST_Valid(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	// ValidateCustom returns an empty Errors map (all valid)
	validateResp, _ := json.Marshal(shared.ValidateCustomResult{Errors: map[int]string{}})
	fc.SetResponse(shared.CmdValidateCustom, shared.Response{Success: true, Data: validateResp})
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/custom", "rules=tcp+dport+80+accept")
	assertRedirect(t, rec, "/custom")
}

// TestHandleCustomPOST_ValidationError exercises the case where core returns
// syntax errors: the handler must re-render the form (HTTP 200, not redirect).
func TestHandleCustomPOST_ValidationError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	// ValidateCustom returns errors for line index 0
	errs := map[int]string{0: "syntax error: invalid expression"}
	validateResp, _ := json.Marshal(shared.ValidateCustomResult{Errors: errs})
	fc.SetResponse(shared.CmdValidateCustom, shared.Response{Success: true, Data: validateResp})

	rec := doAuthFormRequest(t, s, "/custom", "rules=invalid+rule+!!!")
	assertStatus(t, rec, http.StatusOK)
}

// TestHandleCustomPOST_CoreUnavailable exercises the path where ValidateCustom
// returns an error (core unavailable). In that case validation is skipped and
// save is attempted instead.
func TestHandleCustomPOST_CoreUnavailable(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	// ValidateCustom fails
	fc.SetResponse(shared.CmdValidateCustom, errorRespFor("core unavailable"))
	// Save succeeds — validation is skipped on core error
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/custom", "rules=tcp+dport+80+accept")
	assertRedirect(t, rec, "/custom")
}

// TestHandleCustomPOST_CoreError exercises the path where SaveRules fails after
// validation passes.
func TestHandleCustomPOST_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	validateResp, _ := json.Marshal(shared.ValidateCustomResult{Errors: map[int]string{}})
	fc.SetResponse(shared.CmdValidateCustom, shared.Response{Success: true, Data: validateResp})
	fc.SetResponse(shared.CmdSaveRules, errorRespFor("save failed"))

	rec := doAuthFormRequest(t, s, "/custom", "rules=tcp+dport+80+accept")
	assertRedirect(t, rec, "/custom")
}

func TestHandleCustomPOST_EmptyRules(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	// Empty rules: ValidateCustom skips all (no non-blank/non-comment lines),
	// returns empty errors map.
	validateResp, _ := json.Marshal(shared.ValidateCustomResult{Errors: map[int]string{}})
	fc.SetResponse(shared.CmdValidateCustom, shared.Response{Success: true, Data: validateResp})
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/custom", "rules=")
	assertRedirect(t, rec, "/custom")
}
