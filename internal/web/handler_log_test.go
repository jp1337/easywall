package web

import (
	"net/http"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHandleLog_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/log", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleLog_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetLog, successResp([]shared.AuditLogEntry{
		{Time: "2026-04-27T10:00:00Z", Action: "rules_saved", RuleType: "tcp", User: "web"},
	}))

	rec := doAuthRequest(t, s, "GET", "/log", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleLog_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetLog, errorRespFor("unavailable"))

	rec := doAuthRequest(t, s, "GET", "/log", nil)
	assertStatus(t, rec, http.StatusOK)
}
