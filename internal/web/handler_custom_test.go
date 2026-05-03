package web

import (
	"encoding/json"
	"net/http"
	"strings"
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

// ── /custom/validate (HTMX live validation) ────────────────────────────────

func TestHandleCustomValidate_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/custom/validate", "rules=tcp+dport+22+accept")
	assertRedirect(t, rec, "/login")
}

func TestHandleCustomValidate_AllValid(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	resp, _ := json.Marshal(shared.ValidateCustomResult{Errors: map[int]string{}})
	fc.SetResponse(shared.CmdValidateCustom, shared.Response{Success: true, Data: resp})

	rec := doAuthFormRequest(t, s, "/custom/validate", "rules=tcp+dport+22+accept")
	assertStatus(t, rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), "alert-success") {
		t.Errorf("expected alert-success, got: %s", rec.Body.String())
	}
}

func TestHandleCustomValidate_SyntaxErrors(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	resp, _ := json.Marshal(shared.ValidateCustomResult{
		Errors: map[int]string{0: "syntax error: unknown token"},
	})
	fc.SetResponse(shared.CmdValidateCustom, shared.Response{Success: true, Data: resp})

	rec := doAuthFormRequest(t, s, "/custom/validate", "rules=this+is+not+valid")
	assertStatus(t, rec, http.StatusOK)
	body := rec.Body.String()
	if !strings.Contains(body, "alert-error") {
		t.Errorf("expected alert-error, got: %s", body)
	}
	if !strings.Contains(body, "Line 1") {
		t.Errorf("expected 'Line 1' in error list, got: %s", body)
	}
}

func TestHandleCustomValidate_CoreOffline(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdValidateCustom, errorRespFor("core unavailable"))

	rec := doAuthFormRequest(t, s, "/custom/validate", "rules=tcp+dport+22+accept")
	assertStatus(t, rec, http.StatusOK)
	body := rec.Body.String()
	if !strings.Contains(body, "alert-info") {
		t.Errorf("expected alert-info fallback when core is offline, got: %s", body)
	}
	if !strings.Contains(body, "Live validation unavailable") {
		t.Errorf("expected explanatory text, got: %s", body)
	}
}

func TestHandleCustomValidate_EscapesErrorMessage(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	// Core returns an error message that includes HTML — handler must escape it.
	resp, _ := json.Marshal(shared.ValidateCustomResult{
		Errors: map[int]string{0: "<script>alert(1)</script>"},
	})
	fc.SetResponse(shared.CmdValidateCustom, shared.Response{Success: true, Data: resp})

	rec := doAuthFormRequest(t, s, "/custom/validate", "rules=anything")
	body := rec.Body.String()
	if strings.Contains(body, "<script>") {
		t.Errorf("response contains unescaped <script>: %s", body)
	}
}
