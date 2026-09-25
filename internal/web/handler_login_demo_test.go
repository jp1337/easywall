package web

import (
	"net/http"
	"strings"
	"testing"
)

// The demo is entered with a button, not a password. Nothing behind it reaches
// a firewall — the client is the in-memory mock and every credential write is
// refused (TestDemoModeRefusesToWriteCredentials) — so a password guards
// nothing, and typing one was the step that kept every Chrome review waiting on
// a human: an agent does not type passwords into a browser.
func TestTheDemoIsEnteredWithoutAPassword(t *testing.T) {
	s := newDemoTestServer(t)

	page := doRequest(s, http.MethodGet, "/login", nil)
	if !strings.Contains(page.Body.String(), `action="/login/demo"`) {
		t.Fatal("the demo's login page has no way in without a password")
	}
	if strings.Contains(page.Body.String(), `name="password"`) {
		t.Error("the demo's login page still asks for a password: the button is the way in")
	}

	rec := doRequest(s, http.MethodPost, "/login/demo", nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/dashboard" {
		t.Fatalf("POST /login/demo = %d → %q, want 303 → /dashboard", rec.Code, rec.Header().Get("Location"))
	}
	dash := doRequest(s, http.MethodGet, "/dashboard", nil, rec.Result().Cookies()...)
	if dash.Code != http.StatusOK {
		t.Fatalf("the session /login/demo issued does not open the dashboard: %d → %q", dash.Code, dash.Header().Get("Location"))
	}
}

// And nowhere else. On an installation the route does not exist at all, so
// there is no handler whose own check could be the one that is wrong.
func TestOnlyTheDemoHasAWayInWithoutAPassword(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))

	if strings.Contains(doRequest(s, http.MethodGet, "/login", nil).Body.String(), "/login/demo") {
		t.Error("an installation's login page offers the demo's way in")
	}
	if !strings.Contains(doRequest(s, http.MethodGet, "/login", nil).Body.String(), `name="password"`) {
		t.Error("an installation's login page lost its password field")
	}
	rec := doRequest(s, http.MethodPost, "/login/demo", nil)
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionName && c.Value != "" && c.MaxAge >= 0 {
			t.Fatalf("POST /login/demo on an installation issued a session (status %d)", rec.Code)
		}
	}
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /login/demo on an installation = %d, want 404/405: the route must not exist", rec.Code)
	}
}
