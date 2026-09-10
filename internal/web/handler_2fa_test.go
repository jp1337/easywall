package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
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

// Every one of the five routes is behind RequireAuth.
func TestEnrol_EveryRouteRequiresASession(t *testing.T) {
	s := serverWithPassword(t)
	for _, path := range []string{
		"/password/2fa/begin", "/password/2fa/confirm", "/password/2fa/enrol-unverified",
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

// TestEnrol_ConfirmDoesNotReMintCodesWhenItIsNotTheFirstFactor.
//
// A passkey enrolled first may already have minted the operator's one set of
// recovery codes; pairing TOTP as a second factor — or replacing it while it
// is already the only one — must leave that set alone. It is the same rule
// handlePasskeyFinish applies in the other direction, and this was the gap
// before it existed: this path minted a fresh set on every confirm,
// unconditionally.
func TestEnrol_ConfirmDoesNotReMintCodesWhenItIsNotTheFirstFactor(t *testing.T) {
	s := serverWithPassword(t)
	if err := s.passkeys.add("already enrolled", webauthn.Credential{ID: []byte("existing-cred")}); err != nil {
		t.Fatal(err)
	}
	existing := []string{"a-hash-that-must-survive"}
	if err := s.cfg.SaveRecoveryCodes(existing); err != nil {
		t.Fatal(err)
	}

	_, cookie := beginEnrolment(t, s)
	secret := s.pendingSecretFor(t, cookie)
	raw, _ := decodeTOTPSecret(secret)
	code := totpAt(raw, stepAt(time.Now()))

	rec := doFormRequest(s, "POST", "/password/2fa/confirm", "code="+code, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm answered %d, want 200", rec.Code)
	}
	if !s.cfg.TOTPEnabled() {
		t.Fatal("a correct code did not enable the factor")
	}
	if got := s.cfg.RecoveryCodes(); len(got) != 1 || got[0] != existing[0] {
		t.Errorf("recovery codes = %v, want the untouched existing set %v", got, existing)
	}
	if strings.Contains(rec.Body.String(), "recovery-code") {
		t.Error("codes were shown even though none were minted; there is nothing new to show")
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

// failEnrolment submits a code that can never verify — outside the whole
// ±5-minute window matchTOTP searches, not merely eight steps out inside it.
// The shape a board with no RTC produces: !hit, not a diagnosis.
func failEnrolment(t *testing.T, s *Server, cookie *http.Cookie) {
	t.Helper()
	doFormRequest(s, "POST", "/password/2fa/confirm", "code=000000", cookie)
}

// THE test of Task 16. An account that predates this release — password set,
// no factor, the only way that state now arises — is gated to /password by
// RequireSecondFactor. Without this escape, a code that will never verify
// here means an account that exists and cannot be used: the same lockout
// Ruling 11 found in the wizard, one layer later, on an account instead of
// on nothing at all.
func TestEnrol_RecoverAfterAWrongCodeReachesAUsableAccount(t *testing.T) {
	s := serverWithPassword(t)
	_, cookie := beginEnrolment(t, s)
	secret := s.pendingSecretFor(t, cookie)

	failEnrolment(t, s, cookie)
	if s.cfg.TOTPEnabled() {
		t.Fatal("the failed code itself enabled the factor")
	}

	rec := doFormRequest(s, "POST", "/password/2fa/enrol-unverified", "ack=1", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("recover answered %d, want 200 with the codes shown", rec.Code)
	}
	if !s.cfg.TOTPEnabled() {
		t.Fatal("the recovery escape did not enable the factor")
	}
	if s.cfg.TOTPSecret() != secret {
		t.Errorf("stored secret %q does not match the one shown on screen (%q) — a fresh one was minted instead of keeping the pairing", s.cfg.TOTPSecret(), secret)
	}
	if codes := extractRecoveryCodes(rec.Body.String()); len(codes) != recoveryCodeCount {
		t.Errorf("%d recovery codes on the page, want %d", len(codes), recoveryCodeCount)
	}

	// lastCookiePerName, not the raw list: restampSession and setFlash both
	// save the session in this one response, and a browser's jar would keep
	// only the later of the two same-named cookies that produces.
	dash := doRequest(s, "GET", "/dashboard", nil, lastCookiePerName(rec.Result().Cookies())...)
	assertStatus(t, dash, http.StatusOK)
}

// A code inside the band that is diagnosed but never accepted — offset by
// more than totpWindowLogin (±1 step) but still within totpWindowEnrol (±10
// steps). This is a *second* branch from !hit, and Task 7's own wizard fix
// found that a shared switch shape does not mean shared coverage: round 1
// there fixed only !hit, and round 2 was needed because this branch did not
// mark the pending entry failed. Fixed here from the start, but still worth
// its own test rather than trusting !hit's to generalise.
func TestEnrol_RecoverAfterADiagnosedSkewReachesAUsableAccount(t *testing.T) {
	s := serverWithPassword(t)
	_, cookie := beginEnrolment(t, s)
	secret := s.pendingSecretFor(t, cookie)
	raw, err := decodeTOTPSecret(secret)
	if err != nil {
		t.Fatal(err)
	}

	skewed := doFormRequest(s, "POST", "/password/2fa/confirm",
		"code="+totpAt(raw, stepAt(time.Now())+4), cookie)
	if s.cfg.TOTPEnabled() {
		t.Fatal("a diagnosed-but-unaccepted code enabled the factor directly")
	}
	body := strings.ToLower(skewed.Body.String())
	if !strings.Contains(body, "clock") && !strings.Contains(body, "uhr") {
		t.Error("the message does not point at the clock")
	}

	rec := doFormRequest(s, "POST", "/password/2fa/enrol-unverified", "ack=1", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("recover after a diagnosed skew answered %d, want 200 with the codes shown", rec.Code)
	}
	if !s.cfg.TOTPEnabled() {
		t.Fatal("the escape did not enable the factor after a diagnosed-but-unaccepted code")
	}
	if s.cfg.TOTPSecret() != secret {
		t.Error("stored secret does not match the one shown on screen")
	}
	if codes := extractRecoveryCodes(rec.Body.String()); len(codes) != recoveryCodeCount {
		t.Errorf("%d recovery codes on the page, want %d", len(codes), recoveryCodeCount)
	}

	dash := doRequest(s, "GET", "/dashboard", nil, lastCookiePerName(rec.Result().Cookies())...)
	assertStatus(t, dash, http.StatusOK)
}

// Not reachable when a factor already exists: that operator is not locked
// out by a failed code here and can simply leave the page, exactly the
// distinction that makes handler_2fa.go's switch not a dead end in general —
// see Task 16's own report for the case (this one) where it is.
func TestEnrol_RecoverIsNotReachableWithAnExistingFactor(t *testing.T) {
	s := serverWithPassword(t)
	_, hashes, err := newRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.cfg.SaveTOTP("JBSWY3DPEHPK3PXP", hashes); err != nil {
		t.Fatal(err)
	}
	cookie := makeAuthCookie(t, s)

	// A second enrolment attempt — replacing the factor — that then fails.
	doFormRequest(s, "POST", "/password/2fa/begin", "current_password=currentpassword123", cookie)
	failEnrolment(t, s, cookie)

	rec := doFormRequest(s, "POST", "/password/2fa/enrol-unverified", "ack=1", cookie)
	assertRedirect(t, rec, "/password")
	if s.cfg.TOTPSecret() != "JBSWY3DPEHPK3PXP" {
		t.Error("the existing factor was replaced by the escape")
	}
	if n := len(s.cfg.RecoveryCodes()); n != recoveryCodeCount {
		t.Errorf("%d recovery codes stored, want the original %d untouched", n, recoveryCodeCount)
	}
}

// Not reachable before an attempt has failed — offered up front, it would be
// easier than typing the code correctly, and it would stop being an escape
// hatch and start being the path everyone takes.
func TestEnrol_RecoverIsNotReachableBeforeAFailedCode(t *testing.T) {
	s := serverWithPassword(t)
	_, cookie := beginEnrolment(t, s)

	rec := doFormRequest(s, "POST", "/password/2fa/enrol-unverified", "ack=1", cookie)
	if rec.Code != http.StatusOK {
		t.Errorf("recover before any code failed answered %d, want 200 with the setup card re-rendered", rec.Code)
	}
	if s.cfg.TOTPEnabled() {
		t.Fatal("the recovery escape enabled a factor before any code had failed")
	}
}

// The checkbox is the deliberate act, not the click that lands on this
// route. A POST missing it must not enable the factor either, even after a
// code has already failed.
func TestEnrol_RecoverRequiresTheAcknowledgement(t *testing.T) {
	s := serverWithPassword(t)
	_, cookie := beginEnrolment(t, s)
	failEnrolment(t, s, cookie)

	rec := doFormRequest(s, "POST", "/password/2fa/enrol-unverified", "", cookie)
	if s.cfg.TOTPEnabled() {
		t.Fatal("the recovery escape enabled a factor without the acknowledgement")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("recover without ack answered %d, want 200 with the setup card re-rendered", rec.Code)
	}
}

// /password/2fa/recovery — issuing fresh codes for an existing factor —
// refuses while no factor is enrolled at all: codes for an account with
// nothing to recover to are noise, and a password check alone used to hand
// them to a gated operator anyway, which is the loophole review found.
func TestEnrol_RecoveryRefusesWithNoFactorEnrolled(t *testing.T) {
	s := serverWithPassword(t)
	rec := doAuthFormRequest(t, s, "/password/2fa/recovery", "current_password=currentpassword123")
	assertRedirect(t, rec, "/password")
	if n := len(s.cfg.RecoveryCodes()); n != 0 {
		t.Errorf("%d recovery codes issued for an account with no factor", n)
	}
}

// {{if and .Setup.Failed .MustEnrol}} in password.html is the only thing
// between a working handler and an operator who can actually reach it — the
// operator-facing half of this fix round, and the half a Go test over the
// handler alone cannot see at all. Checked against the form's own action
// attribute rather than translated copy, so a locale edit cannot make this
// pass or fail for the wrong reason.
func TestEnrol_TheEscapeCardAppearsExactlyWhenItShould(t *testing.T) {
	const marker = `action="/password/2fa/enrol-unverified"`

	// Absent before any code has failed.
	s1 := serverWithPassword(t)
	before, _ := beginEnrolment(t, s1)
	if strings.Contains(before, marker) {
		t.Error("the escape card is on the page before any code has failed")
	}

	// Present after a code has failed.
	s2 := serverWithPassword(t)
	_, cookie2 := beginEnrolment(t, s2)
	failed := doFormRequest(s2, "POST", "/password/2fa/confirm", "code=000000", cookie2)
	if !strings.Contains(failed.Body.String(), marker) {
		t.Error("the escape card is not on the page after a code failed")
	}

	// Absent with an existing factor, even after a failed code — that
	// operator is not locked out by it and can simply leave the page.
	s3 := serverWithPassword(t)
	_, hashes, err := newRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if err := s3.cfg.SaveTOTP("JBSWY3DPEHPK3PXP", hashes); err != nil {
		t.Fatal(err)
	}
	cookie3 := makeAuthCookie(t, s3)
	doFormRequest(s3, "POST", "/password/2fa/begin", "current_password=currentpassword123", cookie3)
	withFactor := doFormRequest(s3, "POST", "/password/2fa/confirm", "code=000000", cookie3)
	if strings.Contains(withFactor.Body.String(), marker) {
		t.Error("the escape card is on the page even though a factor already exists")
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
	// A second factor standing by, so this test still exercises the password
	// gate rather than colliding with mayRemoveFactor — that guard has its own
	// tests in factors_test.go.
	s.passkeyCount = func() int { return 1 }

	assertRedirect(t, doAuthFormRequest(t, s, "/password/2fa/disable", "current_password=wrong"), "/password")
	if !s.cfg.TOTPEnabled() {
		t.Fatal("a wrong password switched the factor off")
	}

	assertRedirect(t, doAuthFormRequest(t, s, "/password/2fa/disable", "current_password=currentpassword123"), "/password")
	if s.cfg.TOTPEnabled() {
		t.Error("the correct password did not switch the factor off")
	}
	// The passkey standing in above is what makes this reachable at all — see
	// TestEnrol_DisablingKeepsRecoveryCodesWhenAnotherFactorRemains for why the
	// codes must survive it rather than being wiped, as they were before a
	// passkey could be the factor left behind.
	if n := len(s.cfg.RecoveryCodes()); n != recoveryCodeCount {
		t.Errorf("%d recovery hashes survived disabling, want all %d — a factor is still enrolled to use them with", n, recoveryCodeCount)
	}
}

// TestEnrol_DisablingKeepsRecoveryCodesWhenAnotherFactorRemains.
//
// mayRemoveFactor only allows switching TOTP off when another factor —
// necessarily a passkey, since TOTP is the only other kind — is already
// enrolled. Before a passkey could be that other factor, disabling TOTP
// always meant disabling the account's only factor, and clearing the
// recovery codes alongside it was correct: there was nothing left to keep
// them for. Now it silently voids a printout the operator may still need,
// while a factor remains enrolled to use it with. The same class of gap as
// handle2FAConfirm's — see TestEnrol_ConfirmDoesNotReMintCodesWhenItIsNotTheFirstFactor.
func TestEnrol_DisablingKeepsRecoveryCodesWhenAnotherFactorRemains(t *testing.T) {
	s := serverWithPassword(t)
	if err := s.passkeys.add("standing by", webauthn.Credential{ID: []byte("surviving-cred")}); err != nil {
		t.Fatal(err)
	}
	existing := []string{"a-hash-that-must-survive"}
	if err := s.cfg.SaveTOTP("JBSWY3DPEHPK3PXP", existing); err != nil {
		t.Fatal(err)
	}

	assertRedirect(t, doAuthFormRequest(t, s, "/password/2fa/disable", "current_password=currentpassword123"), "/password")
	if s.cfg.TOTPEnabled() {
		t.Fatal("the correct password did not switch the factor off")
	}
	if got := s.cfg.RecoveryCodes(); len(got) != 1 || got[0] != existing[0] {
		t.Errorf("recovery codes = %v, want the untouched existing set %v — a passkey is still enrolled", got, existing)
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
	secret, _, ok := pendingSecretLookup(id)
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
