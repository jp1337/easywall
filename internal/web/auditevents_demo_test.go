package web

import (
	"bytes"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The demo records that somebody signed in, and not from where.
//
// The field is omitted rather than filled with a placeholder: it is already
// optional — the core's addrDetail returns "" for an empty address and the
// demo's handleLogEvent builds no detail at all — so a dash would be one more
// line of code making the same statement.
func TestDemoModeRecordsNoLoginAddress(t *testing.T) {
	a := newAuditEvents(NewDemoClient(), true)
	a.Record(shared.EvLoginOK, "203.0.113.7", 0, false)

	p := <-a.ch
	if p.Addr != "" {
		t.Errorf("the demo queued the address %q; it must record none", p.Addr)
	}
	if p.Event != shared.EvLoginOK {
		t.Errorf("event = %q; the event itself is still recorded", p.Event)
	}
}

// And a real installation still records it. The suppression is demo mode's
// alone; a firewall's audit log without addresses is not an audit log.
func TestAnOrdinaryInstallationStillRecordsTheAddress(t *testing.T) {
	a := newAuditEvents(NewDemoClient(), false)
	a.Record(shared.EvLoginOK, "203.0.113.7", 0, false)

	if p := <-a.ch; p.Addr != "203.0.113.7" {
		t.Errorf("address = %q, want 203.0.113.7", p.Addr)
	}
}

// The demo's log does not carry a visitor's address either. A login refused by
// the rate limit was logged with the full address on stdout, and the demo's
// stdout goes to the host's journal and on to a log server — while
// docs/privacy.md says the application keeps none. A real installation still
// logs it: that line is how an operator sees who is guessing passwords.
func TestTheDemoLogsARefusedLoginWithoutTheAddress(t *testing.T) {
	for _, demo := range []bool{true, false} {
		resetLoginLimiter()
		t.Cleanup(resetLoginLimiter)

		var logs bytes.Buffer
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
		t.Cleanup(func() { slog.SetDefault(prev) })

		var s *Server
		if demo {
			s = newDemoTestServer(t)
		} else {
			s = newTestServer(t, newFakeCore(t))
		}
		for i := 0; i < 6; i++ {
			r := httptest.NewRequest("POST", "/login", strings.NewReader("username=x&password=y"))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.RemoteAddr = "203.0.113.7:41000"
			s.router.ServeHTTP(httptest.NewRecorder(), r)
		}
		slog.SetDefault(prev)

		out := logs.String()
		if !strings.Contains(out, "login rate limit exceeded") {
			t.Fatalf("demo=%v: the refusal was not logged at all:\n%s", demo, out)
		}
		if got := strings.Contains(out, "203.0.113.7"); got == demo {
			t.Errorf("demo=%v: address in the log = %v, want %v:\n%s", demo, got, !demo, out)
		}
	}
}

// The public demo links its operator's legal notice and privacy policy; an
// ordinary installation must not, because that notice names somebody else.
func TestOnlyTheDemoLinksTheLegalNotice(t *testing.T) {
	for _, demo := range []bool{true, false} {
		var s *Server
		if demo {
			s = newDemoTestServer(t)
		} else {
			s = newTestServer(t, newFakeCore(t))
		}
		body := doRequest(s, "GET", "/login", nil).Body.String()
		for _, href := range []string{"https://easywall-project.org/legal-notice/", "https://easywall-project.org/privacy/"} {
			if got := strings.Contains(body, href); got != demo {
				t.Errorf("demo=%v: /login links %s = %v, want %v", demo, href, got, demo)
			}
		}
	}
}
