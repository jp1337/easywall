package web

import (
	"bytes"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// Every route that can write credentials to web.toml, and the form body that
// would make it do so.
//
// A list rather than a pattern: a new credential-writing route has to be added
// here by hand, which is the point. Task 12 adds the four /password/2fa/*
// routes; Task 16 adds /password/2fa/enrol-unverified.
//
// The confirm entry below does NOT exercise handle2FAConfirm's demo guard: with
// no prior begin(), pendingSecretLookup fails and the handler returns via
// totp_setup_expired before ever reaching the IsDemo() check. This list only
// covers begin, disable and recovery for that route.
// TestDemoModeConfirmShowsTheCodesAndStoresNothing below is what actually
// drives confirm's demo branch, because it is the one route whose demo
// behaviour is most visible to a visitor — it puts eight real-looking
// recovery codes on screen.
//
// enrol-unverified's entry below does NOT exercise handle2FAEnrolUnverified's
// IsDemo() guard either, for the same reason confirm's does not: with no
// prior begin(), there is no pending secret, and the totp_setup_expired
// redirect from the earlier !ok check answers first regardless of demo mode
// — deleting the IsDemo() clause leaves this shallow entry passing. Proved,
// not assumed: a review found a comment here once claiming the opposite.
// TestDemoModeEnrolUnverifiedRefusesAfterAFailedCode below drives the real
// path — begin, fail a code, then reach for the escape — so the IsDemo()
// check is the only thing between it and a real secret on disk.
var credentialWritingRoutes = []struct {
	path string
	body string
}{
	// The "!" is load-bearing: without a symbol the new password fails
	// passwordPolicyError before the save, and this entry passed with
	// handlePasswordPOST's demo guard deleted.
	{"/password", "current_password=currentpassword123&new_password=ReplacedInTheDemo1!&confirm_password=ReplacedInTheDemo1!"},
	{"/password/2fa/begin", "current_password=currentpassword123"},
	{"/password/2fa/confirm", "code=000000"},
	{"/password/2fa/enrol-unverified", "ack=1"},
	{"/password/2fa/disable", "current_password=currentpassword123"},
	{"/password/2fa/recovery", "current_password=currentpassword123"},
	// notify_url is a credential too — it is in secretManagedKeys, and
	// configuration.md says it is kept at 0600 beside session_key — and
	// SaveNotifications writes the same real web.toml. Without its guard a demo
	// visitor persisted an address of their choosing, which /notify then
	// rendered back to every other visitor. It is the one entry here that writes
	// none of the three credential fields checked below, so the loop checks the
	// destination as well; a list entry whose mutation produces no output is a
	// hole in this test, not a row in it.
	{"/notify", "kind=webhook&url=https://attacker.example/hook&on_panic=on"},
}

// The public demo runs the whole interface against an in-memory mock, and every
// page is meant to be explorable. web.toml is not in-memory: SaveCredentials
// writes the real file, so a visitor could change the password and lock
// everybody else out — including anyone holding the published demo password —
// until the process restarted.
//
// The five /password/2fa/* routes never touch the password hash at all — they
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
			beforeKind, beforeURL := s.cfg.NotifyDestination()

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
			if kind, addr := s.cfg.NotifyDestination(); kind != beforeKind || addr != beforeURL {
				t.Errorf("%s stored the notification destination %q/%q in demo mode; a visitor "+
					"to the public demo points this installation's notifications at themselves",
					tc.path, kind, addr)
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
		"current_password=currentpassword123&new_password=ReplacedInTheDemo1!&confirm_password=ReplacedInTheDemo1!",
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

// The enumerated list above cannot reach handle2FAConfirm's demo branch: with no
// prior begin, pendingSecretLookup fails and the handler returns before the
// IsDemo check. So the one route whose demo behaviour is most visible to a
// visitor — it shows eight real-looking recovery codes — would have no coverage
// at all. This drives the real sequence.
func TestDemoModeConfirmShowsTheCodesAndStoresNothing(t *testing.T) {
	s := newDemoTestServer(t)
	hash, err := HashPassword("currentpassword123")
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.Password = hash

	cookie := makeAuthCookie(t, s)
	if rec := doFormRequest(s, "POST", "/password/2fa/begin",
		"current_password=currentpassword123", cookie); rec.Code != http.StatusOK {
		t.Fatalf("begin answered %d, want 200", rec.Code)
	}

	secret := s.pendingSecretFor(t, cookie)
	raw, err := decodeTOTPSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	rec := doFormRequest(s, "POST", "/password/2fa/confirm",
		"code="+totpAt(raw, stepAt(time.Now())), cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm answered %d, want 200 with the codes shown", rec.Code)
	}

	if s.cfg.TOTPEnabled() {
		t.Error("the demo enrolled a second factor; the next visitor cannot get in")
	}
	if n := len(s.cfg.RecoveryCodes()); n != 0 {
		t.Errorf("the demo stored %d recovery hashes", n)
	}

	// The visitor still sees the whole feature — that is the point of running the
	// flow rather than refusing at the door.
	body := rec.Body.String()
	shown := 0
	for _, tok := range strings.Fields(body) {
		if isRecoveryShape(strings.Trim(tok, "<>\"")) {
			shown++
		}
	}
	if shown < recoveryCodeCount {
		t.Errorf("%d recovery codes on the demo page, want %d", shown, recoveryCodeCount)
	}
}

// The shallow credentialWritingRoutes entry for /password/2fa/enrol-unverified
// never reaches its own IsDemo() guard — see the comment on that list for
// why. This test drives the path that does: begin, fail a code so the escape
// unlocks, then take it. In a real installation this is exactly the sequence
// that stores a second factor; in the demo it must store nothing.
func TestDemoModeEnrolUnverifiedRefusesAfterAFailedCode(t *testing.T) {
	s := newDemoTestServer(t)
	hash, err := HashPassword("currentpassword123")
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.Password = hash

	cookie := makeAuthCookie(t, s)
	if rec := doFormRequest(s, "POST", "/password/2fa/begin",
		"current_password=currentpassword123", cookie); rec.Code != http.StatusOK {
		t.Fatalf("begin answered %d, want 200", rec.Code)
	}
	doFormRequest(s, "POST", "/password/2fa/confirm", "code=000000", cookie)

	rec := doFormRequest(s, "POST", "/password/2fa/enrol-unverified", "ack=1", cookie)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("enrol-unverified in demo mode answered %d, want a redirect", rec.Code)
	}
	if s.cfg.TOTPEnabled() {
		t.Error("the demo enrolled a second factor through the recovery escape; the next visitor cannot get in")
	}
	if n := len(s.cfg.RecoveryCodes()); n != 0 {
		t.Errorf("the demo stored %d recovery hashes through the escape", n)
	}
}

// /notify saves over HTMX, so its refusal is an HX-Trigger rather than the
// flash TestDemoModeSaysWhyItRefused reads off /password. Same key, other
// transport — and the key, not a sentence of English copy, because the copy is
// edited and the key is the stable artefact.
func TestTheDemoSaysWhyItWillNotSaveTheNotificationSettings(t *testing.T) {
	s := newDemoTestServer(t)
	enrollFactor(t, s)
	rec := doAuthFormHTMX(t, s, "/notify", "kind=webhook&url=https://attacker.example/hook")
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "demo_readonly") {
		t.Errorf("HX-Trigger = %q, want demo_readonly: a form that silently does nothing "+
			"reads as a broken page", trigger)
	}
}

// demoKitchenSinkForm switches on every field a web.toml-writing handler reads,
// so one body drives each of them past its validation to the write.
const demoKitchenSinkForm = "current_password=currentpassword123" +
	"&new_password=ReplacedInTheDemo1!&confirm_password=ReplacedInTheDemo1!" +
	"&code=000000&ack=1" +
	"&kind=webhook&url=https://attacker.example/hook&on_panic=on&on_rolled_back=on&on_accepted=on&on_failed_logins=on" +
	"&telemetry=on&answer=yes"

// The list above is written by hand; this is the part nobody has to remember.
// It posts to every POST route the demo registers and asserts that web.toml is
// byte-identical afterwards — demo.md promises a visitor's changes live in
// memory and are gone on restart, and web.toml is the one file that survives
// one. /system/telemetry was not on the list and wrote telemetry = true into
// it: 303 to the visitor, the next visitor's counting decided.
//
// Twice per route, the second time with reset=1, because a reset is a second
// submit inside the same form and takes a branch of its own.
func TestDemoModeWritesNothingToWebTOML(t *testing.T) {
	s := newDemoTestServer(t)
	routes := map[string]bool{}
	if err := chi.Walk(s.router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodPost {
			routes[strings.ReplaceAll(strings.ReplaceAll(route, "{", ""), "}", "")] = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !routes["/system/telemetry"] {
		t.Fatal("the walk did not find /system/telemetry; it is not walking the demo's router")
	}
	for route := range routes {
		for _, body := range []string{demoKitchenSinkForm, demoKitchenSinkForm + "&reset=1"} {
			s := newDemoTestServer(t)
			hash, err := HashPassword("currentpassword123")
			if err != nil {
				t.Fatal(err)
			}
			s.cfg.Password = hash
			before, err := os.ReadFile(s.cfg.configPath)
			if err != nil {
				t.Fatal(err)
			}
			doAuthFormRequest(t, s, route, body)
			if after, _ := os.ReadFile(s.cfg.configPath); !bytes.Equal(before, after) {
				t.Errorf("POST %s (reset=%v) rewrote web.toml in demo mode; the change outlives the "+
					"restart demo.md says clears it", route, strings.HasSuffix(body, "&reset=1"))
			}
		}
	}
}
