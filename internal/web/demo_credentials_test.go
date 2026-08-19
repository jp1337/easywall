package web

import (
	"net/http"
	"strings"
	"testing"
)

// Every route that can write credentials to web.toml, and the form body that
// would make it do so.
//
// A list rather than a pattern: a new credential-writing route has to be added
// here by hand, which is the point. Task 12 adds the four /password/2fa/* routes.
var credentialWritingRoutes = []struct {
	path string
	body string
}{
	{"/password", "current_password=currentpassword123&new_password=ReplacedInTheDemo1&confirm_password=ReplacedInTheDemo1"},
}

// The public demo runs the whole interface against an in-memory mock, and every
// page is meant to be explorable. web.toml is not in-memory: SaveCredentials
// writes the real file, so a visitor could change the password and lock
// everybody else out — including anyone holding the published demo password —
// until the process restarted.
func TestDemoModeRefusesToWriteCredentials(t *testing.T) {
	for _, tc := range credentialWritingRoutes {
		t.Run(tc.path, func(t *testing.T) {
			s := newDemoTestServer(t)
			hash, err := HashPassword("currentpassword123")
			if err != nil {
				t.Fatal(err)
			}
			s.cfg.Password = hash
			before := s.cfg.PasswordHash()

			rec := doAuthFormRequest(t, s, tc.path, tc.body)
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("%s answered %d, want a redirect carrying the refusal", tc.path, rec.Code)
			}
			if after := s.cfg.PasswordHash(); after != before {
				t.Errorf("%s changed the stored credential in demo mode; a visitor to the "+
					"public demo can lock everybody else out", tc.path)
			}
		})
	}
}

// And the refusal has to say so. A form that silently does nothing reads as a
// broken page, which is worse for a demo than a plain sentence.
func TestDemoModeSaysWhyItRefused(t *testing.T) {
	s := newDemoTestServer(t)
	hash, _ := HashPassword("currentpassword123")
	s.cfg.Password = hash

	cookie := makeAuthCookie(t, s)
	_ = doFormRequest(s, "POST", "/password",
		"current_password=currentpassword123&new_password=ReplacedInTheDemo1&confirm_password=ReplacedInTheDemo1",
		cookie)
	rec := doRequest(s, "GET", "/password", nil, cookie)
	if !strings.Contains(rec.Body.String(), "demo") && !strings.Contains(rec.Body.String(), "Demo") {
		t.Error("the demo refusal is not on the page; the form appears to do nothing at all")
	}
}
