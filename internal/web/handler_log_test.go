package web

import (
	"net/http"
	"strings"
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

func TestHandleLogFilter_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/log/filter?q=foo", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleLogFilter_Empty(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetLog, successResp([]shared.AuditLogEntry{}))

	rec := doAuthRequest(t, s, "GET", "/log/filter?q=anything", nil)
	assertStatus(t, rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), "No matching entries") {
		t.Errorf("expected empty-state row, got: %s", rec.Body.String())
	}
}

func TestHandleLogFilter_NoQueryReturnsAll(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetLog, successResp([]shared.AuditLogEntry{
		{Time: "t1", Action: "rules_saved", User: "web"},
		{Time: "t2", Action: "options_saved", User: "alice"},
	}))

	rec := doAuthRequest(t, s, "GET", "/log/filter", nil)
	assertStatus(t, rec, http.StatusOK)
	body := rec.Body.String()
	// The label, not the stored identifier: the table shows "Rules saved", and
	// that is what an operator reads and searches for.
	if !strings.Contains(body, "Rules saved") || !strings.Contains(body, "Options saved") {
		t.Errorf("expected both rows in response, got: %s", body)
	}
}

func TestHandleLogFilter_MatchByAction(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetLog, successResp([]shared.AuditLogEntry{
		{Time: "t1", Action: "rules_saved", User: "web"},
		{Time: "t2", Action: "options_saved", User: "alice"},
		{Time: "t3", Action: "rules_applied", User: "web"},
	}))

	rec := doAuthRequest(t, s, "GET", "/log/filter?q=options", nil)
	body := rec.Body.String()
	if !strings.Contains(body, "Options saved") {
		t.Errorf("expected options_saved in body, got: %s", body)
	}
	if strings.Contains(body, "Rules saved") || strings.Contains(body, "rules_applied") {
		t.Errorf("non-matching rows leaked into filtered result: %s", body)
	}
}

func TestHandleLogFilter_MatchByUser(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetLog, successResp([]shared.AuditLogEntry{
		{Time: "t1", Action: "rules_saved", User: "web"},
		{Time: "t2", Action: "rules_saved", User: "alice"},
	}))

	rec := doAuthRequest(t, s, "GET", "/log/filter?q=alice", nil)
	body := rec.Body.String()
	if !strings.Contains(body, "alice") {
		t.Errorf("expected 'alice' in filtered body, got: %s", body)
	}
	// 'web' should not appear as a column value any more.
	if strings.Contains(body, ">web<") {
		t.Errorf("non-matching user row leaked through: %s", body)
	}
}

func TestHandleLogFilter_CaseInsensitive(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetLog, successResp([]shared.AuditLogEntry{
		{Time: "t1", Action: "RULES_APPLIED", User: "web"},
	}))

	rec := doAuthRequest(t, s, "GET", "/log/filter?q=rules", nil)
	// An action the label map does not know is humanised rather than printed as a
	// snake_case token, so the row reads as language even before the map catches up.
	if !strings.Contains(rec.Body.String(), "RULES APPLIED") {
		t.Errorf("expected case-insensitive match, got: %s", rec.Body.String())
	}
}

func TestHandleLogFilter_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetLog, errorRespFor("unavailable"))

	// Core failure should still return 200 + empty-state row, not 500.
	rec := doAuthRequest(t, s, "GET", "/log/filter?q=foo", nil)
	assertStatus(t, rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), "No matching entries") {
		t.Errorf("expected fallback row on core error, got: %s", rec.Body.String())
	}
}
