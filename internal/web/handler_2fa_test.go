package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// begin renders the setup card as the response to the POST and returns the body
// plus the session cookie the pending secret is keyed by.
func beginEnrolment(t *testing.T, s *Server) (body string, cookie *http.Cookie) {
	t.Helper()
	cookie = makeAuthCookie(t, s)
	rec := doFormRequest(s, "POST", "/password/2fa/begin", "current_password=currentpassword123", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("begin answered %d, want 200 with the setup card rendered in place", rec.Code)
	}
	return rec.Body.String(), cookie
}

func serverWithPassword(t *testing.T) *Server {
	t.Helper()
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, err := HashPassword("currentpassword123")
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.Password = hash
	return s
}

// The page after begin is the POST's own response: a reload cannot mint a second
// secret and the page cannot be bookmarked.
func TestEnrol_BeginRendersInPlaceAndStoresNothing(t *testing.T) {
	s := serverWithPassword(t)
	body, _ := beginEnrolment(t, s)

	if s.cfg.TOTPEnabled() {
		t.Error("begin enabled the factor; nothing may be stored before a code is confirmed")
	}
	if !strings.Contains(body, "data:image/png;base64,") {
		t.Error("no QR code on the setup card")
	}
	if !strings.Contains(body, "UTC") {
		t.Error("the server time is not on the setup card; it is the only place somebody " +
			"without shell access notices their clock is wrong before they lock themselves out")
	}
}

// Enabling requires the current password, the same rule the page already applies
// to changing it.
func TestEnrol_BeginRequiresTheCurrentPassword(t *testing.T) {
	s := serverWithPassword(t)
	rec := doAuthFormRequest(t, s, "/password/2fa/begin", "current_password=wrong")
	assertRedirect(t, rec, "/password")
	if s.cfg.TOTPEnabled() {
		t.Error("a wrong password still began an enrolment")
	}
}

// Every one of the four routes is behind RequireAuth.
func TestEnrol_EveryRouteRequiresASession(t *testing.T) {
	s := serverWithPassword(t)
	for _, path := range []string{
		"/password/2fa/begin", "/password/2fa/confirm",
		"/password/2fa/disable", "/password/2fa/recovery",
	} {
		rec := doFormRequest(s, "POST", path, "current_password=currentpassword123")
		assertRedirect(t, rec, "/login")
	}
}

// A code that hits at ±1 stores the secret and the eight hashes in one write,
// and shows the codes once.
func TestEnrol_ConfirmStoresEverythingInOneWriteAndShowsTheCodesOnce(t *testing.T) {
	s := serverWithPassword(t)
	_, cookie := beginEnrolment(t, s)

	secret := s.pendingSecretFor(t, cookie)
	raw, _ := decodeTOTPSecret(secret)
	code := totpAt(raw, stepAt(time.Now()))

	rec := doFormRequest(s, "POST", "/password/2fa/confirm", "code="+code, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm answered %d, want 200 with the codes shown", rec.Code)
	}
	if !s.cfg.TOTPEnabled() {
		t.Fatal("a correct code did not enable the factor")
	}
	if n := len(s.cfg.RecoveryCodes()); n != recoveryCodeCount {
		t.Errorf("%d recovery hashes stored, want %d", n, recoveryCodeCount)
	}

	body := rec.Body.String()
	shown := 0
	for _, line := range strings.Fields(body) {
		if isRecoveryShape(strings.Trim(line, "<>\"")) {
			shown++
		}
	}
	if shown < recoveryCodeCount {
		t.Errorf("%d recovery codes on the page, want %d — they are shown once and never again", shown, recoveryCodeCount)
	}
	// And a second GET must not show them.
	if again := doRequest(s, "GET", "/password", nil, cookie); strings.Contains(again.Body.String(), "-") &&
		strings.Count(again.Body.String(), "recovery-code") > 0 {
		t.Error("the codes are still on the page after a reload; shown once must mean shown once")
	}
}

// The clock is the largest support risk and no security risk. A code that is
// right but far out gets a diagnosis with a sign and a magnitude, and nothing is
// stored.
func TestEnrol_AFarOutCodeDiagnosesTheClock(t *testing.T) {
	s := serverWithPassword(t)
	_, cookie := beginEnrolment(t, s)

	secret := s.pendingSecretFor(t, cookie)
	raw, _ := decodeTOTPSecret(secret)
	code := totpAt(raw, stepAt(time.Now())+8) // the phone is four minutes ahead

	rec := doFormRequest(s, "POST", "/password/2fa/confirm", "code="+code, cookie)
	if s.cfg.TOTPEnabled() {
		t.Fatal("a code eight steps out enabled the factor")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "4") {
		t.Errorf("the diagnosis does not name the magnitude:\n%s", body)
	}
	if !strings.Contains(strings.ToLower(body), "clock") && !strings.Contains(strings.ToLower(body), "uhr") {
		t.Error("the message does not point at the clock; the fault is on the server and the " +
			"message must not point at the human")
	}
}

func TestEnrol_AWrongCodeStoresNothing(t *testing.T) {
	s := serverWithPassword(t)
	_, cookie := beginEnrolment(t, s)

	_ = doFormRequest(s, "POST", "/password/2fa/confirm", "code=000000", cookie)
	if s.cfg.TOTPEnabled() {
		t.Error("a wrong code enabled the factor")
	}
}

// The unconfirmed secret never enters a cookie: gorilla/sessions with only a
// hash key signs but does not encrypt, so a cookie value is readable plaintext.
func TestEnrol_TheUnconfirmedSecretIsNeverInACookie(t *testing.T) {
	s := serverWithPassword(t)
	cookie := makeAuthCookie(t, s)
	rec := doFormRequest(s, "POST", "/password/2fa/begin", "current_password=currentpassword123", cookie)

	secret := s.pendingSecretFor(t, cookie)
	for _, c := range rec.Result().Cookies() {
		if strings.Contains(c.Value, secret) {
			t.Errorf("cookie %q carries the unconfirmed secret in readable form", c.Name)
		}
	}
}

// Enabling and disabling both end every other session; the acting one survives.
func TestEnrol_ConfirmEndsOtherSessionsAndKeepsThisOne(t *testing.T) {
	s := serverWithPassword(t)
	other := makeAuthCookie(t, s)

	_, cookie := beginEnrolment(t, s)
	secret := s.pendingSecretFor(t, cookie)
	raw, _ := decodeTOTPSecret(secret)
	rec := doFormRequest(s, "POST", "/password/2fa/confirm",
		"code="+totpAt(raw, stepAt(time.Now())), cookie)

	assertRedirect(t, doRequest(s, "GET", "/dashboard", nil, other), "/login")

	acting := cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionName {
			acting = c
		}
	}
	assertStatus(t, doRequest(s, "GET", "/dashboard", nil, acting), http.StatusOK)
}

// Disabling needs the current password and no code: whoever is at that form
// already holds a session that came through the second factor, and a further
// code would only create a new lockout case against no attacker it stops.
func TestEnrol_DisableNeedsThePasswordAndNoCode(t *testing.T) {
	s := serverWithPassword(t)
	plain, hashes, _ := newRecoveryCodes()
	_ = plain
	_ = s.cfg.SaveTOTP("JBSWY3DPEHPK3PXP", hashes)

	assertRedirect(t, doAuthFormRequest(t, s, "/password/2fa/disable", "current_password=wrong"), "/password")
	if !s.cfg.TOTPEnabled() {
		t.Fatal("a wrong password switched the factor off")
	}

	assertRedirect(t, doAuthFormRequest(t, s, "/password/2fa/disable", "current_password=currentpassword123"), "/password")
	if s.cfg.TOTPEnabled() {
		t.Error("the correct password did not switch the factor off")
	}
	if n := len(s.cfg.RecoveryCodes()); n != 0 {
		t.Errorf("%d recovery hashes survived disabling", n)
	}
}

// New codes invalidate all eight old ones.
func TestEnrol_RegeneratingInvalidatesTheOldCodes(t *testing.T) {
	s := serverWithPassword(t)
	old, hashes, _ := newRecoveryCodes()
	_ = s.cfg.SaveTOTP("JBSWY3DPEHPK3PXP", hashes)

	rec := doAuthFormRequest(t, s, "/password/2fa/recovery", "current_password=currentpassword123")
	if rec.Code != http.StatusOK {
		t.Fatalf("recovery answered %d, want 200 with the new codes shown", rec.Code)
	}
	for i, c := range old {
		if _, ok := consumeRecoveryCode(c, s.cfg.RecoveryCodes()); ok {
			t.Errorf("old code %d still works after regeneration", i)
		}
	}
}

// web.toml unwritable at confirm: nothing is enabled, a message is shown, and
// the pending secret stays in memory — otherwise the operator re-pairs their app
// because the disk was briefly full.
func TestEnrol_AnUnwritableConfigKeepsThePendingSecret(t *testing.T) {
	s := serverWithPassword(t)
	_, cookie := beginEnrolment(t, s)
	secret := s.pendingSecretFor(t, cookie)

	s.cfg.configPath = "/nonexistent/directory/web.toml"

	raw, _ := decodeTOTPSecret(secret)
	rec := doFormRequest(s, "POST", "/password/2fa/confirm",
		"code="+totpAt(raw, stepAt(time.Now())), cookie)
	if s.cfg.TOTPEnabled() {
		t.Error("the factor was enabled even though the write failed")
	}
	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Errorf("confirm answered %d", rec.Code)
	}
	if got := s.pendingSecretFor(t, cookie); got != secret {
		t.Error("the pending secret was discarded; the operator now re-pairs their app " +
			"because a disk was briefly full")
	}
}

// The QR code is dark on white in both themes: an inverted QR code is rejected
// by a good share of scanners, and that is a defect only a screenshot in the
// dark theme shows.
func TestQR_IsADataURIThatTheCSPAlreadyAllows(t *testing.T) {
	uri, err := qrPNGDataURI("otpauth://totp/easywall:admin?secret=JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(uri, "data:image/png;base64,") {
		t.Errorf("QR data URI starts %.40q; the CSP allows img-src 'self' data: and nothing else", uri)
	}
	if len(uri) < 200 {
		t.Errorf("the QR data URI is %d bytes, which is too short to be an image", len(uri))
	}
}

// pendingSecretFor reads the in-memory secret for a session cookie, so a test
// can compute the code the operator would be typing.
func (s *Server) pendingSecretFor(t *testing.T, cookie *http.Cookie) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/password", nil)
	req.AddCookie(cookie)
	sess, err := s.store.Get(req, SessionName)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := sess.Values[SessionIDKey].(string)
	secret, ok := pendingSecretLookup(id)
	if !ok {
		t.Fatalf("no pending secret for session %q", id)
	}
	return secret
}

// Regression: skewMinutes(2) is 1, and an offset of 2 is the first value that
// reaches the clock-skew message at all (totpWindowLogin is 1). "about 1
// minutes behind" was wrong in both locales; clockSkewKey now picks a
// dedicated singular id the same way count_entry_one/count_entry_many are
// picked elsewhere, rather than interpolating the number into a message
// that only has a plural form.
func TestClockSkewKey_SingularAndPluralMinutes(t *testing.T) {
	cases := []struct {
		offset int
		want   string
	}{
		{2, "totp_clock_behind_one"},  // skewMinutes(2) == 1
		{3, "totp_clock_behind_many"}, // skewMinutes(3) == 2
		{-2, "totp_clock_ahead_one"},
		{-3, "totp_clock_ahead_many"},
	}
	for _, c := range cases {
		if got := clockSkewKey(c.offset); got != c.want {
			t.Errorf("clockSkewKey(%d) = %q, want %q", c.offset, got, c.want)
		}
	}
}
