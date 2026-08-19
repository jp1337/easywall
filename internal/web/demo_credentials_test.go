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
	{"/password/2fa/begin", "current_password=currentpassword123"},
	{"/password/2fa/confirm", "code=000000"},
	{"/password/2fa/disable", "current_password=currentpassword123"},
	{"/password/2fa/recovery", "current_password=currentpassword123"},
}

// The public demo runs the whole interface against an in-memory mock, and every
// page is meant to be explorable. web.toml is not in-memory: SaveCredentials
// writes the real file, so a visitor could change the password and lock
// everybody else out — including anyone holding the published demo password —
// until the process restarted.
//
// The four /password/2fa/* routes never touch the password hash at all — they
// write totp_secret and recovery_codes — so a check of PasswordHash() alone
// would pass here whether or not their own demo guard existed, which is not a
// test of them. TOTPSecret and RecoveryCodes are checked for the same reason
// PasswordHash is: they are the other fields SaveTOTP and SaveRecoveryCodes
// would have written to web.toml.
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
			beforeTOTP := s.cfg.TOTPSecret()
			beforeRecovery := len(s.cfg.RecoveryCodes())

			rec := doAuthFormRequest(t, s, tc.path, tc.body)
			if rec.Code != http.StatusSeeOther && rec.Code != http.StatusOK {
				t.Fatalf("%s answered %d", tc.path, rec.Code)
			}
			if after := s.cfg.PasswordHash(); after != before {
				t.Errorf("%s changed the stored credential in demo mode; a visitor to the "+
					"public demo can lock everybody else out", tc.path)
			}
			if after := s.cfg.TOTPSecret(); after != beforeTOTP {
				t.Errorf("%s changed the stored TOTP secret in demo mode", tc.path)
			}
			if after := len(s.cfg.RecoveryCodes()); after != beforeRecovery {
				t.Errorf("%s changed the stored recovery codes in demo mode", tc.path)
			}
		})
	}
}

// And the refusal has to say so. A form that silently does nothing reads as a
// broken page, which is worse for a demo than a plain sentence.
//
// Two things this test has to get right, both found the hard way by a
// reviewer who deleted the guard in handlePasswordPOST and watched this test
// keep passing:
//
//  1. The flash lives in a signed cookie, set on the POST's own response
//     (setFlash writes the Set-Cookie for that response, not for whatever
//     cookie the request carried in). Reusing the pre-POST makeAuthCookie
//     cookie for the follow-up GET carries no flash at all — the GET has to
//     reuse the cookies the POST actually set.
//  2. The assertion has to name text that is not also true of the page
//     regardless of the guard. render() sets PageData.Demo unconditionally
//     in demo mode, base.html always draws a "Demo" chip in the topbar when
//     that's set, and password.html includes the topbar — so asserting on
//     "demo"/"Demo" passes whether or not the refusal ever fired. "not saved"
//     appears only in the demo_readonly translation itself.
func TestDemoModeSaysWhyItRefused(t *testing.T) {
	s := newDemoTestServer(t)
	hash, _ := HashPassword("currentpassword123")
	s.cfg.Password = hash

	post := doFormRequest(s, "POST", "/password",
		"current_password=currentpassword123&new_password=ReplacedInTheDemo1&confirm_password=ReplacedInTheDemo1",
		makeAuthCookie(t, s))

	cookies := post.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("the refusal set no cookie, so it carried no flash")
	}
	rec := doRequest(s, "GET", "/password", nil, cookies...)

	// Text unique to demo_readonly, not shared with the demo chip.
	if !strings.Contains(rec.Body.String(), "not saved") {
		t.Error("the demo refusal is not on the page; the form appears to do nothing at all")
	}
}
