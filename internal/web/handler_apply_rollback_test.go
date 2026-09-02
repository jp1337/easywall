package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// A rollback that lands after the window closed undid nothing — the previous
// rules came back on their own. Saying "rolled back" for it is the same class
// of lie as "accepted and applied successfully" after a timeout: it tells the
// operator they acted at the one moment they did not.
func TestApplyRollback_TooLateIsNotReportedAsSuccess(t *testing.T) {
	s := newDemoTestServer(t) // demo-backed; no window is open

	rec := doAuthFormRequest(t, s, "/apply/rollback", "")
	assertRedirect(t, rec, "/apply")

	cookie := rec.Result().Cookies()
	if len(cookie) == 0 {
		t.Fatal("expected a flash cookie explaining what happened")
	}
	follow := doRequest(s, "GET", "/apply", nil, cookie[0], makeAuthCookie(t, s))
	body := follow.Body.String()
	if strings.Contains(body, "The rules were rolled back") {
		t.Error("the interface must not report a rollback for a request that came too late")
	}
	if !strings.Contains(body, "nothing was rolled back") {
		t.Errorf("expected the page to say the window had already closed; body was:\n%s", body)
	}
}

func TestApplyRollback_RollsBackAnOpenWindow(t *testing.T) {
	s := newDemoTestServer(t)

	if err := s.client.SaveRules("tcp", []shared.PortRule{{Port: "8443"}}); err != nil {
		t.Fatalf("SaveRules: %v", err)
	}
	if err := s.client.ApplyRules(); err != nil {
		t.Fatalf("ApplyRules: %v", err)
	}

	rec := doAuthFormRequest(t, s, "/apply/rollback", "")
	assertRedirect(t, rec, "/apply")

	cookie := rec.Result().Cookies()
	if len(cookie) == 0 {
		t.Fatal("expected a flash cookie explaining what happened")
	}
	follow := doRequest(s, "GET", "/apply", nil, cookie[0], makeAuthCookie(t, s))
	body := follow.Body.String()
	if !strings.Contains(body, "The rules were rolled back") {
		t.Errorf("expected the page to say the rules were rolled back; body was:\n%s", body)
	}

	status, err := s.client.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status.Acceptance != shared.AcceptanceRolledBack {
		t.Errorf("acceptance after a rollback = %q, want %q", status.Acceptance, shared.AcceptanceRolledBack)
	}
	if status.AcceptanceRemaining != 0 {
		t.Errorf("AcceptanceRemaining after a rollback = %d, want 0", status.AcceptanceRemaining)
	}
}

func TestApplyRollback_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdCancelAcceptance, errorRespFor("rollback error"))

	rec := doAuthFormRequest(t, s, "/apply/rollback", "")
	assertRedirect(t, rec, "/apply")
}

func TestApplyRollback_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "POST", "/apply/rollback", nil)
	assertRedirect(t, rec, "/login")
}

func TestApplyRollback_RequiresPOST(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doAuthRequest(t, s, http.MethodGet, "/apply/rollback", nil)
	assertStatus(t, rec, http.StatusMethodNotAllowed)
}

// The audit detail is a token, translated. Without an entry in auditDetailKeys
// the log view would show the core's raw English token "cancelled by
// operator" sitting untranslated beside eight translated ones.
func TestAuditDetailKeys_HasTheOperatorRollbackEntry(t *testing.T) {
	if _, ok := auditDetailKeys["cancelled by operator"]; !ok {
		t.Error("auditDetailKeys has no entry for the operator-rollback detail, so the log " +
			"view shows the core's raw token beside translated ones")
	}
}
