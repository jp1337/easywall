package web

import (
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func TestHandlePasswordGET_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/password", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandlePasswordGET_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doAuthRequest(t, s, "GET", "/password", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlePasswordPOST_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/password", "current_password=x&new_password=y&confirm_password=y")
	assertRedirect(t, rec, "/login")
}

func TestHandlePasswordPOST_WrongCurrent(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	// cfg.Password is "" — VerifyPassword("wrong", "") returns false
	rec := doAuthFormRequest(t, s, "/password",
		"current_password=wrong&new_password=ValidPassword123!&confirm_password=ValidPassword123!")
	assertRedirect(t, rec, "/password")
}

func TestHandlePasswordPOST_TooShort(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	// Set a real hash so the current password check passes
	hash, _ := HashPassword("currentpassword123")
	s.cfg.Password = hash

	rec := doAuthFormRequest(t, s, "/password",
		"current_password=currentpassword123&new_password=short&confirm_password=short")
	assertRedirect(t, rec, "/password")
}

func TestHandlePasswordPOST_Mismatch(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword("currentpassword123")
	s.cfg.Password = hash

	rec := doAuthFormRequest(t, s, "/password",
		"current_password=currentpassword123&new_password=ValidPassword123!&confirm_password=DifferentPassword123")
	assertRedirect(t, rec, "/password")
}

func TestHandlePasswordPOST_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword("currentpassword123")
	s.cfg.Password = hash

	rec := doAuthFormRequest(t, s, "/password",
		"current_password=currentpassword123&new_password=ValidPassword123!&confirm_password=ValidPassword123!")
	assertRedirect(t, rec, "/password")
}

// Changing the password ends every other session.
//
// Sessions are a signed cookie with nothing to revoke on the server, so a
// change used to leave anyone already signed in exactly where they were until
// the session timed out — including in the case the change is usually made for.
func TestHandlePasswordPOST_EndsSessionsIssuedUnderTheOldPassword(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	hash, err := HashPassword("currentpassword123")
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.Password = hash

	// Somebody else is signed in, in another browser.
	other := makeAuthCookie(t, s)
	if rec := doRequest(s, "GET", "/dashboard", nil, other); rec.Code == http.StatusSeeOther {
		t.Fatal("the other session is valid before the change")
	}

	rec := doAuthFormRequest(t, s, "/password",
		"current_password=currentpassword123&new_password=ValidPassword123!&confirm_password=ValidPassword123!")
	assertRedirect(t, rec, "/password")

	after := doRequest(s, "GET", "/dashboard", nil, other)
	if after.Code != http.StatusSeeOther || after.Header().Get("Location") != "/login" {
		t.Errorf("the other session must be refused after the password change, got %d %q",
			after.Code, after.Header().Get("Location"))
	}
}

// The operator making the change stays signed in.
func TestHandlePasswordPOST_KeepsTheChangersOwnSession(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	hash, err := HashPassword("currentpassword123")
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.Password = hash

	cookie := makeAuthCookie(t, s)
	rec := doFormRequest(s, "POST", "/password",
		"current_password=currentpassword123&new_password=ValidPassword123!&confirm_password=ValidPassword123!",
		cookie)
	assertRedirect(t, rec, "/password")

	// The response re-issues the session; the browser would send that one next.
	refreshed := rec.Result().Cookies()
	if len(refreshed) == 0 {
		t.Fatal("expected a refreshed session cookie")
	}
	if next := doRequest(s, "GET", "/dashboard", nil, refreshed[0]); next.Code == http.StatusSeeOther {
		t.Error("the operator who changed the password must stay signed in")
	}
}

// The password hash is read on every authenticated request — RequireAuth
// compares the session's fingerprint against it — and written when the operator
// changes their password. Different goroutines, and without a lock that is a
// race on a string header: a reader can see the new length with the old
// pointer. Under -race this test is what fails if the lock goes away.
func TestConfig_PasswordChangeDoesNotRaceWithRequests(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, err := HashPassword("currentpassword123")
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.Password = hash
	cookie := makeAuthCookie(t, s)

	// Hashed once, for the reason written out in
	// TestSaveFirstRun_SecondSetupCannotTakeOverTheAccount: argon2id at
	// m=64 MiB inside a goroutine, times fifteen, is a gigabyte of live heap
	// for the race detector to shadow, and what this test is about is
	// SaveCredentials writing a torn pair, not the KDF.
	newHash, err := HashPassword("anotherpassword123")
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 15; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			doRequest(s, "GET", "/dashboard", nil, cookie)
		}()
		go func() {
			defer wg.Done()
			_ = s.cfg.IsFirstRun()
			_ = s.cfg.PasswordHash()
		}()
		go func() {
			defer wg.Done()
			_ = s.cfg.SaveCredentials("admin", newHash)
		}()
	}
	wg.Wait()

	// Whatever the interleaving, the stored credentials must be a matched pair.
	user, stored := s.cfg.Credentials()
	if user != "admin" {
		t.Errorf("username came out as %q", user)
	}
	if !VerifyPassword("anotherpassword123", stored) && !VerifyPassword("currentpassword123", stored) {
		t.Error("the stored hash matches neither password; it was torn")
	}
}

// TestRecoveryCodesAreOfferedToAPasskeyOnlyAccount pins the condition that
// decides whether an operator can see, and replace, the eight codes that are
// their last way in.
//
// It lived inside the second-factor card's {{if .TOTPEnabled}} until 2.18,
// which was correct while TOTP was the only factor there was. Adding a second
// kind of factor made it wrong in a way nothing could see: the handler already
// gated /password/2fa/recovery on hasSecondFactor(), so the route worked — the
// form was simply never rendered, and locales/en.json's own "0 left — make new
// ones under Password" sent the operator to a control that was not there.
//
// Asserted on the rendered page rather than on the data struct, because the
// defect was in the template and a data-level test would have stayed green
// through all of it.
func TestRecoveryCodesAreOfferedToAPasskeyOnlyAccount(t *testing.T) {
	for _, tc := range []struct {
		name    string
		totp    string
		passkey bool
		want    bool
	}{
		{"no factor at all", "", false, false},
		{"TOTP only", "JBSWY3DPEHPK3PXP", false, true},
		{"passkey only", "", true, true},
		{"both", "JBSWY3DPEHPK3PXP", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t, newFakeCore(t))
			if tc.totp != "" {
				if err := s.cfg.SaveTOTP(tc.totp, nil); err != nil {
					t.Fatal(err)
				}
			}
			if tc.passkey {
				if err := s.passkeys.add("a key", webauthn.Credential{ID: []byte("cred-1")}); err != nil {
					t.Fatal(err)
				}
			}

			rec := doAuthRequest(t, s, "GET", "/password", nil)
			assertStatus(t, rec, http.StatusOK)
			got := strings.Contains(rec.Body.String(), `action="/password/2fa/recovery"`)
			if got != tc.want {
				t.Fatalf("the New codes form is present=%v, want %v — an account with a "+
					"factor and no way to replace its recovery codes is one lost device "+
					"from having no way in", got, tc.want)
			}
		})
	}
}

// TestThePasswordPageHasNoTwoCardsWithOneName is the sibling of
// TestTheSystemPageHasNoTwoButtonsWithOneName, and it exists because the
// same mistake was made again on this page one commit later.
//
// Moving the recovery-code controls out of the second-factor card gave the new
// card the title the *fresh codes* card already had, so the response that
// issues eight codes rendered two cards called "Recovery codes", stacked. No
// test saw it; a screenshot did. The template now hides the controls on that
// one response, and this asserts the outcome rather than the condition — a
// third card added later with either title fails here too.
//
// Driven at the response that carries the codes, because that is the only
// state in which the duplicate appeared.
func TestThePasswordPageHasNoTwoCardsWithOneName(t *testing.T) {
	s := serverWithPassword(t)

	// The confirm response, not a plain GET: .Codes is non-nil on exactly one
	// response in the application's life, and that is the only state in which
	// the two cards ever appeared. Written as a GET first, this test passed
	// against the unfixed template.
	_, cookie := beginEnrolment(t, s)
	secret := s.pendingSecretFor(t, cookie)
	raw, err := decodeTOTPSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	rec := doFormRequest(s, "POST", "/password/2fa/confirm",
		"code="+totpAt(raw, stepAt(time.Now())), cookie)
	assertStatus(t, rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), "recovery-code") {
		t.Fatal("the confirm response carries no recovery codes; this test is " +
			"no longer looking at the response the duplicate appeared on")
	}
	titles := sectionTitles(rec.Body.String())
	if len(titles) < 3 {
		t.Fatalf("found %d section titles on /password (%v); the test is no longer "+
			"looking at the right page", len(titles), titles)
	}
	seen := map[string]int{}
	for _, ti := range titles {
		seen[ti]++
	}
	for title, n := range seen {
		if n > 1 {
			t.Errorf("the title %q heads %d cards; a reader cannot tell which one "+
				"holds what", title, n)
		}
	}
}

// sectionTitles returns the visible text of every card heading in html.
func sectionTitles(html string) []string {
	var out []string
	re := regexp.MustCompile(`(?s)<h2[^>]*class="[^"]*section-title[^"]*"[^>]*>(.*?)</h2>`)
	tags := regexp.MustCompile(`<[^>]*>`)
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		text := strings.Join(strings.Fields(tags.ReplaceAllString(m[1], " ")), " ")
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}
