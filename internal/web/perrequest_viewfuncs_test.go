package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// TestPerRequestViewFuncsFollowTheRequestsLanguage guards the *rebinding
// mechanism* in render()/renderPartial() — not actionLabel specifically.
// Both actionLabel and lastUsed are registered once in viewFuncs (a stub
// that only keeps ParseGlob happy at startup) and then rebound per request,
// inside the same tmpl.Funcs(...) block, with that request's own tFunc. If
// either rebinding were ever hoisted out of that block — bound once at
// startup or lazily on first use — it would capture whichever request's
// language happened to bind first, and every subsequent visitor, in any
// language, would see that one frozen in.
//
// actionLabel is the only one of the two reachable through a rendered page
// today (via handleLogFilter -> renderPartial("log_rows", ...) ->
// log.html's {{actionLabel .Action}}); lastUsed has no template wired to it
// yet (Task 11). So this test exercises actionLabel end to end, against a
// single Server instance, across two requests that differ only in their
// language cookie — but the property it protects belongs to both.
// Whoever adds a third per-request view function is extending the same
// mechanism and inherits this test too.
func TestPerRequestViewFuncsFollowTheRequestsLanguage(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetLog, successResp([]shared.AuditLogEntry{
		{Time: "t1", Action: "rules_saved", User: "web"},
	}))
	auth := makeAuthCookie(t, s)

	en := doRequest(s, "GET", "/log/filter", nil, auth)
	if !strings.Contains(en.Body.String(), "Rules saved") {
		t.Fatalf("plain request rendered %q, want the English label %q",
			en.Body.String(), "Rules saved")
	}

	// Same server, same handler, only the language cookie differs.
	de := doRequest(s, "GET", "/log/filter", nil, auth,
		&http.Cookie{Name: LangCookie, Value: "de"})
	if !strings.Contains(de.Body.String(), "Regeln gespeichert") {
		t.Errorf("de-cookie request rendered %q, want the German label %q — "+
			"the language did not change between requests on the same server, "+
			"which means a per-request view function is capturing a stale tFunc",
			de.Body.String(), "Regeln gespeichert")
	}
}
