package web

import (
	"net/url"
	"strings"
	"testing"
)

// The page's whole job: what the operator typed is what the file holds, and
// what the file holds is what the page shows them next time. Both halves in
// one test, because a form that stores correctly and renders an empty field
// back is indistinguishable from one that stored nothing.
func TestSavingTheNotificationSettingsStoresThemAndRendersThemBack(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))
	// The second-factor gate sends an unenrolled session to /password before a
	// handler inside the protected group ever runs — see RequireSecondFactor.
	enrollFactor(t, s)
	form := url.Values{
		"kind": {"ntfy"}, "url": {"https://ntfy.example/t"},
		"on_rolled_back": {"on"}, "on_panic": {"on"},
	}
	rr := doAuthFormRequest(t, s, "/notify", form.Encode())
	if rr.Code >= 400 {
		t.Fatalf("POST /notify = %d", rr.Code)
	}
	if s.cfg.NotifyKind != "ntfy" || !s.cfg.NotifyOnRolledBack || s.cfg.NotifyOnAccepted || !s.cfg.NotifyOnPanic {
		t.Fatalf("not stored: %+v", s.cfg.WebConfig)
	}
	body := doRequest(s, "GET", "/notify", nil, makeAuthCookie(t, s)).Body.String()
	if !strings.Contains(body, "https://ntfy.example/t") {
		t.Error("the saved URL is not rendered back on the page")
	}
}

// file:// is not a web endpoint, and the notifier would hand it to an
// http.Client that cannot dial it. Refused before it reaches the file.
func TestAURLThatIsNotHTTPIsRefused(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))
	enrollFactor(t, s)
	form := url.Values{"kind": {"webhook"}, "url": {"file:///etc/passwd"}}
	_ = doAuthFormRequest(t, s, "/notify", form.Encode())
	if s.cfg.NotifyURL == "file:///etc/passwd" {
		t.Fatal("a non-HTTP scheme was stored")
	}
}
