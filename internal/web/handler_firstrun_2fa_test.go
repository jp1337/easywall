package web

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// beginFirstRun submits a valid step 1 and returns the body of the step-2
// page plus the cookies that carry the pending id. Step 1 has no checkbox
// left to tick — the TOTP step is unconditional — so a valid submission always
// lands here.
func beginFirstRun(t *testing.T, s *Server) (string, []*http.Cookie) {
	t.Helper()
	rec := doFormRequest(s, "POST", "/firstrun",
		"username=admin&password=firstrunpassword1!&password_confirm=firstrunpassword1!"+
			"&ssh_port=22&ipv6_mode=filter")
	if rec.Code != http.StatusOK {
		t.Fatalf("step 1 answered %d, want 200 with the setup step rendered in place", rec.Code)
	}
	return rec.Body.String(), rec.Result().Cookies()
}

func firstRunPendingSecret(t *testing.T, s *Server, cookies []*http.Cookie) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/firstrun", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	sess, err := s.store.Get(req, SessionName)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := sess.Values[firstRunPendingKey].(string)
	p, ok := firstRunPendingLookup(id)
	if !ok {
		t.Fatalf("no pending first run for id %q", id)
	}
	return p.Secret
}

// Nothing is written until a code confirms. If this ever stops holding, an
// abandoned wizard leaves an account nobody can sign into.
func TestFirstRun2FA_StepOneStoresNothing(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	body, _ := beginFirstRun(t, s)

	if !s.cfg.IsFirstRun() {
		t.Error("step 1 created the account; the wizard is closed before a code was seen")
	}
	if !strings.Contains(body, "data:image/png;base64,") {
		t.Error("no QR code on the setup step")
	}
	if !strings.Contains(body, "UTC") {
		t.Error("the server time is not shown; a wrong clock is the one failure this " +
			"page exists to make visible")
	}
}

// The whole point of the change.
func TestFirstRun2FA_ConfirmCreatesTheAccountWithTheFactor(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)
	// Without this, GetSettings inside applyFirstRunChoices fails to parse the
	// fakeCore's default empty response, staging the choices never succeeds,
	// and completeFirstRun redirects instead of rendering the codes — the same
	// setup every other staging-path test in handler_firstrun_test.go already
	// carries, for the same reason.
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))

	_, cookies := beginFirstRun(t, s)
	raw, err := decodeTOTPSecret(firstRunPendingSecret(t, s, cookies))
	if err != nil {
		t.Fatal(err)
	}

	rec := doFormRequest(s, "POST", "/firstrun/confirm",
		"code="+totpAt(raw, stepAt(time.Now())), cookies...)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm answered %d, want 200 with the codes shown", rec.Code)
	}

	if s.cfg.IsFirstRun() {
		t.Fatal("a confirmed code did not create the account")
	}
	if !s.cfg.TOTPEnabled() {
		t.Error("the account was created without the factor that was just confirmed")
	}
	if n := len(s.cfg.RecoveryCodes()); n != recoveryCodeCount {
		t.Errorf("%d recovery hashes stored, want %d", n, recoveryCodeCount)
	}

	shown := 0
	for _, tok := range strings.Fields(rec.Body.String()) {
		if isRecoveryShape(strings.Trim(tok, "<>\"")) {
			shown++
		}
	}
	if shown < recoveryCodeCount {
		t.Errorf("%d recovery codes on the page, want %d — they are shown once", shown, recoveryCodeCount)
	}
}

// The invariant this fix round exists for: if the account was written, the
// codes reach the operator — whatever happened to the staging. This is the
// feature's own target environment: a board whose core socket is not up yet
// on first boot. The operator ticks the box, confirms a valid code, and must
// not be left believing only that some ports need setting by hand while
// holding a second factor whose recovery codes were never shown.
func TestFirstRun2FA_ConfirmShowsCodesEvenWhenStagingFails(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveRules, errorRespFor("the core is not up yet"))

	_, cookies := beginFirstRun(t, s)
	raw, err := decodeTOTPSecret(firstRunPendingSecret(t, s, cookies))
	if err != nil {
		t.Fatal(err)
	}

	rec := doFormRequest(s, "POST", "/firstrun/confirm",
		"code="+totpAt(raw, stepAt(time.Now())), cookies...)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm answered %d, want 200 with the codes shown despite the staging failure", rec.Code)
	}

	if s.cfg.IsFirstRun() {
		t.Fatal("a confirmed code did not create the account even though only the staging failed")
	}
	if !s.cfg.TOTPEnabled() {
		t.Error("the account was created without the factor that was just confirmed")
	}

	shown := 0
	for _, tok := range strings.Fields(rec.Body.String()) {
		if isRecoveryShape(strings.Trim(tok, "<>\"")) {
			shown++
		}
	}
	if shown < recoveryCodeCount {
		t.Errorf("%d recovery codes on the page, want %d — a staging failure must not "+
			"cost the operator their only look at a second factor's recovery codes", shown, recoveryCodeCount)
	}

	if !strings.Contains(rec.Body.String(), "recovery codes are below") {
		t.Error("the page does not say the initial choices could not be staged — the " +
			"operator is left thinking everything worked")
	}
}

// The wizard collects more than the account. Confirming a factor must not drop
// the ports and the IPv6 mode the operator chose above the password.
func TestFirstRun2FA_ConfirmStillStagesTheOtherAnswers(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	var savedTCP *shared.Command
	fc.OnCommand(shared.CmdSaveRules, func(c shared.Command) { savedTCP = &c })

	_, cookies := beginFirstRun(t, s)
	raw, _ := decodeTOTPSecret(firstRunPendingSecret(t, s, cookies))
	_ = doFormRequest(s, "POST", "/firstrun/confirm",
		"code="+totpAt(raw, stepAt(time.Now())), cookies...)

	if savedTCP == nil {
		t.Fatal("the ports were never staged; applyFirstRunChoices did not run")
	}
	if !strings.Contains(string(savedTCP.Payload), "\"22\"") {
		t.Errorf("the SSH port is not in the staged rules: %s", savedTCP.Payload)
	}
}

// A failed write during confirm must not cost the operator their pairing: the
// pending entry survives, the setup step is re-rendered with the same secret,
// and the message names the disk rather than the core — SaveFirstRun writes
// web.toml and never touches the core socket. Reverting the fix that made
// completeFirstRun report a write failure back to handleFirstRunConfirm turns
// this red: the handler falls through to the redirect-to-/firstrun response
// that used to run unconditionally, the pending entry is left stranded behind
// a step 1 GET that cannot reach it, and the retry below mints a fresh secret
// instead of reusing the one already scanned.
func TestFirstRun2FA_ConfirmSurvivesAFailedWrite(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	_, cookies := beginFirstRun(t, s)
	raw, err := decodeTOTPSecret(firstRunPendingSecret(t, s, cookies))
	if err != nil {
		t.Fatal(err)
	}

	// Make the write fail without touching the pending entry itself.
	s.cfg.configPath = "/nonexistent/directory/web.toml"

	rec := doFormRequest(s, "POST", "/firstrun/confirm",
		"code="+totpAt(raw, stepAt(time.Now())), cookies...)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm answered %d, want 200 with the setup step re-rendered", rec.Code)
	}
	if !s.cfg.IsFirstRun() {
		t.Fatal("the account was created despite the write failing")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "data:image/png;base64,") {
		t.Error("the setup step was not re-rendered after the failed write; the operator " +
			"was sent back to step 1 instead and the pairing is now unreachable")
	}
	lower := strings.ToLower(body)
	if !strings.Contains(lower, "disk") {
		t.Error("the message does not name the disk — SaveFirstRun never reaches the core")
	}
	if strings.Contains(lower, "core connection") {
		t.Error("the message still blames the core connection (save_error), not the disk")
	}

	// The pairing survives: the pending entry is still there under the same id,
	// still holding the very secret already scanned into the phone.
	stillThere, ok := firstRunPendingLookupForTest(t, s, cookies)
	if !ok {
		t.Fatal("the pending entry was lost after the failed write")
	}
	if decodeAgain, derr := decodeTOTPSecret(stillThere.Secret); derr != nil || string(decodeAgain) != string(raw) {
		t.Error("the surviving entry's secret no longer matches the one already scanned")
	}

	// Fix the disk and retry with a fresh code from the very same secret: the
	// operator must not have to re-pair.
	s.cfg.configPath = t.TempDir() + "/web.toml"

	rec2 := doFormRequest(s, "POST", "/firstrun/confirm",
		"code="+totpAt(raw, stepAt(time.Now())), cookies...)
	if rec2.Code != http.StatusOK {
		t.Fatalf("the retry after the disk recovered answered %d", rec2.Code)
	}
	if s.cfg.IsFirstRun() {
		t.Error("the retry did not create the account even though the disk recovered")
	}
	if !s.cfg.TOTPEnabled() {
		t.Error("the retry lost the second factor paired before the first failure")
	}
}

// firstRunPendingLookupForTest is a thin wrapper so the survival check above
// reads the same session/cookie plumbing firstRunPendingSecret already uses.
func firstRunPendingLookupForTest(t *testing.T, s *Server, cookies []*http.Cookie) (pendingFirstRun, bool) {
	t.Helper()
	req := httptest.NewRequest("GET", "/firstrun", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	sess, err := s.store.Get(req, SessionName)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := sess.Values[firstRunPendingKey].(string)
	return firstRunPendingLookup(id)
}

// A code that is right against a clock that is not.
func TestFirstRun2FA_AFarOutCodeDiagnosesTheClockAndStoresNothing(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	_, cookies := beginFirstRun(t, s)
	raw, _ := decodeTOTPSecret(firstRunPendingSecret(t, s, cookies))

	rec := doFormRequest(s, "POST", "/firstrun/confirm",
		"code="+totpAt(raw, stepAt(time.Now())+8), cookies...)

	if !s.cfg.IsFirstRun() {
		t.Fatal("a code eight steps out created the account")
	}
	body := strings.ToLower(rec.Body.String())
	if !strings.Contains(body, "clock") && !strings.Contains(body, "uhr") {
		t.Error("the message does not point at the clock; the fault is on the server " +
			"and the message must not point at the human")
	}
	// firstrun.html renders its own flash block rather than the shared "flash"
	// template base.html and password.html use, and that hand-rolled copy once
	// called {{T .Flash}} without threading .FlashN through — so this message,
	// which is entirely built from a totp_clock_*_{one,many} key plus {{.N}},
	// rendered "about <no value> minute(s)" on this exact page while the
	// identical string worked fine reached via /password. Guard against that
	// regression coming back.
	if strings.Contains(body, "no value") {
		t.Error("the clock-skew flash rendered \"<no value>\" instead of the minute " +
			"count; firstrun.html's flash block must pass (dict \"N\" .FlashN) to T, " +
			"the same way base.html's shared \"flash\" template does")
	}
}

func TestFirstRun2FA_AWrongCodeStoresNothing(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	_, cookies := beginFirstRun(t, s)
	_ = doFormRequest(s, "POST", "/firstrun/confirm", "code=000000", cookies...)

	if !s.cfg.IsFirstRun() {
		t.Error("a wrong code created the account")
	}
}

// lastCookiePerName mimics an actual browser's cookie jar rather than
// httptest's raw Set-Cookie list: a handler that calls Save on the same
// session twice in one response (firstRunError does, once to stash the
// answers and again inside setFlash) emits two Set-Cookie headers for the
// same name, and a browser's jar keeps only the later one. Re-sending every
// header verbatim, as the other redirect-then-GET tests in this package do,
// happens to work for them only because they never check anything that was
// added by the *later* of the two writes — this test does.
func lastCookiePerName(cookies []*http.Cookie) []*http.Cookie {
	byName := make(map[string]*http.Cookie, len(cookies))
	var order []string
	for _, c := range cookies {
		if _, ok := byName[c.Name]; !ok {
			order = append(order, c.Name)
		}
		byName[c.Name] = c
	}
	out := make([]*http.Cookie, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}

// The escape hatch exists precisely for an operator who cannot make a code
// verify. It must not itself go silent: an entry aged past
// firstRunPendingLifetime has to say so, in both routes that can meet it, and
// the username has to survive the trip back to step 1 — a blank wizard after
// ten minutes of setup work is its own kind of dead end.
func TestFirstRun2FA_ExpiredEntryIsNamedAndKeepsTheAnswers(t *testing.T) {
	for _, path := range []string{"/firstrun/confirm", "/firstrun/recover"} {
		t.Run(path, func(t *testing.T) {
			fc := newFakeCore(t)
			s := newFirstRunTestServer(t, fc)

			_, cookies := beginFirstRun(t, s)

			req := httptest.NewRequest("GET", "/firstrun", nil)
			for _, c := range cookies {
				req.AddCookie(c)
			}
			sess, err := s.store.Get(req, SessionName)
			if err != nil {
				t.Fatal(err)
			}
			id, _ := sess.Values[firstRunPendingKey].(string)
			p, ok := firstRunPendingLookup(id)
			if !ok {
				t.Fatal("no pending entry to age")
			}
			// Backdate the same entry rather than fabricate a new one, so the
			// answers it carries are exactly what step 1 collected.
			firstRunPendingStore(id, pendingFirstRun{
				Answers:      p.Answers,
				PasswordHash: p.PasswordHash,
				Secret:       p.Secret,
				Issued:       time.Now().Add(-firstRunPendingLifetime - time.Second),
			})

			form := "code=000000"
			if path == "/firstrun/recover" {
				form = "ack=1"
			}
			rec := doFormRequest(s, "POST", path, form, cookies...)
			assertRedirect(t, rec, "/firstrun")

			if !s.cfg.IsFirstRun() {
				t.Fatal("an expired entry created the account")
			}

			back := doRequest(s, "GET", "/firstrun", nil, lastCookiePerName(rec.Result().Cookies())...)
			body := back.Body.String()
			if !strings.Contains(strings.ToLower(body), "timed out") {
				t.Error("the expired entry produced no flash at all; want totp_setup_expired")
			}
			if !strings.Contains(body, `value="admin"`) {
				t.Error("the username was not kept across the expired escape hatch — the " +
					"operator would have to retype everything as well as start the pairing over")
			}
		})
	}
}

// Without a pending id the routes create nothing and send the operator back.
func TestFirstRun2FA_NoPendingSetupStartsAgain(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	for _, path := range []string{"/firstrun/confirm", "/firstrun/recover"} {
		form := "code=000000"
		if path == "/firstrun/recover" {
			form = "ack=1"
		}
		rec := doFormRequest(s, "POST", path, form)
		assertRedirect(t, rec, "/firstrun")
		if !s.cfg.IsFirstRun() {
			t.Fatalf("%s created an account with no pending setup behind it", path)
		}
	}
}

// The routes exist only while the wizard does.
func TestFirstRun2FA_RoutesAreGoneOnceAnAccountExists(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc) // has a password, so IsFirstRun() is false

	for _, path := range []string{"/firstrun/confirm", "/firstrun/recover"} {
		rec := doFormRequest(s, "POST", path, "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s answered %d on a configured install, want 404 — a "+
				"credential-writing route must not outlive the wizard", path, rec.Code)
		}
	}
}

// TestTheWizardHasNoWayPastTheSecondFactor asserts that the first-run wizard
// cannot produce an account without one.
//
// It offered a checkbox and a Skip button. An operator who reads nothing and
// presses the button ended up with a firewall interface reachable over the
// network behind one argon2id hash, which is the thing 2.18 is for.
func TestTheWizardHasNoWayPastTheSecondFactor(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	// The route is gone, not merely unlinked. An unlinked route is a route.
	if rec := doFormRequest(s, "POST", "/firstrun/skip", ""); rec.Code != http.StatusNotFound {
		t.Errorf("POST /firstrun/skip answered %d; the skip route still exists", rec.Code)
	}

	// Nor does omitting the old checkbox get you past it: there is no checkbox
	// left, and a valid step 1 leads to the TOTP step every time.
	rec := doFormRequest(s, "POST", "/firstrun",
		"password="+testPassword+"&password_confirm="+testPassword+
			"&username=admin&ssh_port=22")
	body := strings.ToLower(rec.Body.String())
	if !strings.Contains(body, "totp") && !strings.Contains(body, "authenticator") {
		t.Error("submitting the wizard did not lead to the second-factor step")
	}
	if !s.cfg.IsFirstRun() {
		t.Error("an account was created before a factor was confirmed")
	}
	if s.cfg.TOTPSecret() != "" {
		t.Error("a secret was stored before the operator confirmed a code")
	}
}

// wizardRenderableStrings returns every firstrun_* locale entry for lang — the
// substrings the wizard can actually put in front of an operator. The
// totp_clock_* keys are deliberately out of scope: Task 6 owns them, edits
// them in the same locale files, and a check here would fail on work that is
// not this test's to police.
func wizardRenderableStrings(t *testing.T, lang string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	for id, text := range localeStrings(t, lang) {
		if strings.HasPrefix(id, "firstrun_") {
			out[id] = text
		}
	}
	return out
}

// TestTheWizardDoesNotPromiseALaterThatDoesNotExist asserts no string the wizard
// can render still offers to defer the second factor.
//
// Six strings survived the commit that made the factor mandatory, in both
// locales, each true until that moment. A passing test suite said nothing,
// because no test reads copy for whether it is still true.
func TestTheWizardDoesNotPromiseALaterThatDoesNotExist(t *testing.T) {
	// Substrings that only make sense when declining is possible. English and
	// German, because a check in one locale is half a check.
	forbidden := []string{
		"later under Password",
		"without a second factor",
		"password alone",
		"später unter Passwort",
		"ohne zweiten Faktor",
		"nur mit einem Passwort",
	}
	for _, locale := range []string{"en", "de"} {
		for id, text := range wizardRenderableStrings(t, locale) {
			for _, bad := range forbidden {
				if strings.Contains(text, bad) {
					t.Errorf("%s/%s still offers to defer the second factor: %q contains %q",
						locale, id, text, bad)
				}
			}
		}
	}
}

// recoveryCodeListItem matches one <li class="recovery-code">CODE</li> from
// the template. Anchored to that element rather than reusing the
// whitespace-token-plus-isRecoveryShape trick the count-only tests above use:
// that trick also matches "btn-primary" and "management" elsewhere on this
// very page (dashes stripped and re-inserted, then within the recovery
// alphabet once I/L/O/U are folded the way normaliseRecoveryCode folds them),
// which only costs those tests an inflated count but would cost this one a
// wrong code to sign in with.
var recoveryCodeListItem = regexp.MustCompile(`(?s)<li class="recovery-code">\s*([0-9A-Z]{5}-[0-9A-Z]{5})\s*</li>`)

// extractRecoveryCodes pulls the plain codes out of a rendered recovery-codes
// page, in the order they were issued.
func extractRecoveryCodes(body string) []string {
	var codes []string
	for _, m := range recoveryCodeListItem.FindAllStringSubmatch(body, -1) {
		codes = append(codes, m[1])
	}
	return codes
}

// failFirstRunCode submits a code that can never verify — not a mismatched
// digit, eight steps out like AFarOutCodeDiagnosesTheClockAndStoresNothing
// above, but outside the ±5-minute window matchTOTP searches entirely. It is
// the shape a board with no RTC produces: the clock is not offset, it is
// wrong by years, and no window search reaches the operator's real code.
func failFirstRunCode(t *testing.T, s *Server, cookies []*http.Cookie) {
	t.Helper()
	doFormRequest(s, "POST", "/firstrun/confirm", "code=000000", cookies...)
}

// THE test of this fix round. A board with no RTC boots years off, a code
// against that clock can never verify, and deleting the skip path in the
// commit above this one closed the only way such a board used to get an
// account at all. The recovery-code escape is what replaces it: the account
// is written with the secret already on screen, not a blank one — the
// authenticator just paired works the moment the clock is fixed, without
// re-enrolling.
func TestFirstRun2FA_RecoverAfterAFailedCodeCreatesTheAccountWithTheFactor(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))

	_, cookies := beginFirstRun(t, s)
	secret := firstRunPendingSecret(t, s, cookies)

	failFirstRunCode(t, s, cookies)
	if !s.cfg.IsFirstRun() {
		t.Fatal("the failed code itself created the account")
	}

	rec := doFormRequest(s, "POST", "/firstrun/recover", "ack=1", cookies...)
	if rec.Code != http.StatusOK {
		t.Fatalf("recover answered %d, want 200 with the codes shown", rec.Code)
	}
	if s.cfg.IsFirstRun() {
		t.Fatal("the recovery escape did not create the account")
	}
	if s.cfg.TOTPSecret() != secret {
		t.Errorf("stored secret %q does not match the one shown on screen (%q) — a fresh one was minted instead of keeping the pairing", s.cfg.TOTPSecret(), secret)
	}
	codes := extractRecoveryCodes(rec.Body.String())
	if len(codes) != recoveryCodeCount {
		t.Errorf("%d recovery codes on the page, want %d", len(codes), recoveryCodeCount)
	}
}

// A code inside the band that is diagnosed but never accepted — offset by
// more than totpWindowLogin (±1 step, ±30 seconds) but still within
// totpWindowEnrol (±10 steps, ±5 minutes) — used to be as dead an end as a
// flatly wrong code: handleFirstRunConfirm's clock-skew case told the
// operator the truth about their clock and then, unlike the !hit case right
// above it in the same switch, never marked the pending entry Failed. Found
// on review by rendering the flow with a *correct* code at a moderate offset
// rather than a wrong one, which is exactly why it needs its own test instead
// of trusting the !hit coverage to generalise.
func TestFirstRun2FA_ADiagnosedSkewAlsoUnlocksTheEscape(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))

	_, cookies := beginFirstRun(t, s)
	secret := firstRunPendingSecret(t, s, cookies)
	raw, err := decodeTOTPSecret(secret)
	if err != nil {
		t.Fatal(err)
	}

	// Four steps out: inside totpWindowEnrol (diagnosed), outside
	// totpWindowLogin (never accepted here) — the band this fix closes.
	rec := doFormRequest(s, "POST", "/firstrun/confirm",
		"code="+totpAt(raw, stepAt(time.Now())+4), cookies...)
	if !s.cfg.IsFirstRun() {
		t.Fatal("a diagnosed-but-unaccepted code created the account directly")
	}
	body := strings.ToLower(rec.Body.String())
	if !strings.Contains(body, "clock") && !strings.Contains(body, "uhr") {
		t.Error("the message does not point at the clock")
	}

	rec2 := doFormRequest(s, "POST", "/firstrun/recover", "ack=1", cookies...)
	if rec2.Code != http.StatusOK {
		t.Fatalf("recover after a diagnosed skew answered %d, want 200 with the codes shown", rec2.Code)
	}
	if s.cfg.IsFirstRun() {
		t.Fatal("the escape did not open after a diagnosed-but-unaccepted code")
	}
	if s.cfg.TOTPSecret() != secret {
		t.Error("the stored secret does not match the one shown on screen")
	}
	if codes := extractRecoveryCodes(rec2.Body.String()); len(codes) != recoveryCodeCount {
		t.Errorf("%d recovery codes on the page, want %d", len(codes), recoveryCodeCount)
	}
}

// The clock-independent way in the ruling asks for: a code from the escape's
// own recovery codes signs in, regardless of what the clock says.
func TestFirstRun2FA_ARecoveryCodeFromTheEscapeSignsIn(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))

	_, cookies := beginFirstRun(t, s)
	failFirstRunCode(t, s, cookies)
	rec := doFormRequest(s, "POST", "/firstrun/recover", "ack=1", cookies...)
	codes := extractRecoveryCodes(rec.Body.String())
	if len(codes) == 0 {
		t.Fatal("no recovery codes to sign in with")
	}

	first := doFormRequest(s, "POST", "/login", "username=admin&password=firstrunpassword1!")
	verify := doFormRequest(s, "POST", "/login/verify", "code="+codes[0], first.Result().Cookies()...)
	assertRedirect(t, verify, "/dashboard")
}

// The gate opens because a factor is enrolled — not because a recovery code
// got the operator past it this once. RequireSecondFactor asks hasFactor(),
// and hasFactor is true the moment TOTPSecret is non-empty; the recovery code
// that got this session in does not itself count as one (Task 5's
// factorCount deliberately does not count recovery codes).
func TestFirstRun2FA_RecoverOpensTheGate(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))

	_, cookies := beginFirstRun(t, s)
	failFirstRunCode(t, s, cookies)
	rec := doFormRequest(s, "POST", "/firstrun/recover", "ack=1", cookies...)
	codes := extractRecoveryCodes(rec.Body.String())
	if len(codes) == 0 {
		t.Fatal("no recovery codes to sign in with")
	}

	first := doFormRequest(s, "POST", "/login", "username=admin&password=firstrunpassword1!")
	verify := doFormRequest(s, "POST", "/login/verify", "code="+codes[0], first.Result().Cookies()...)
	assertRedirect(t, verify, "/dashboard")

	// lastCookiePerName, not the raw list: granting the session and consuming
	// the recovery code both save it in this one response, and a browser's jar
	// would keep only the later of the two same-named cookies that produces.
	dash := doRequest(s, "GET", "/dashboard", nil, lastCookiePerName(verify.Result().Cookies())...)
	assertStatus(t, dash, http.StatusOK)
}

// The escape is not reachable before a code has failed — offered up front, it
// would be easier than typing the code correctly, and it would stop being an
// escape hatch and start being the path everyone takes.
func TestFirstRun2FA_RecoverIsNotReachableBeforeAFailedCode(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	_, cookies := beginFirstRun(t, s)
	rec := doFormRequest(s, "POST", "/firstrun/recover", "ack=1", cookies...)

	if !s.cfg.IsFirstRun() {
		t.Fatal("the recovery escape created an account before any code had failed")
	}
	if s.cfg.TOTPEnabled() {
		t.Error("a factor was enrolled before any code had failed")
	}
	if strings.Contains(rec.Body.String(), "recovery-code") {
		t.Error("recovery codes were shown before any code had failed")
	}
}

// The checkbox is the deliberate act, not the click that lands on this route.
// A POST missing it must not create the account either, even after a code
// has already failed.
func TestFirstRun2FA_RecoverRequiresTheAcknowledgement(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	_, cookies := beginFirstRun(t, s)
	failFirstRunCode(t, s, cookies)
	rec := doFormRequest(s, "POST", "/firstrun/recover", "", cookies...)

	if !s.cfg.IsFirstRun() {
		t.Fatal("the recovery escape created an account without the acknowledgement")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("recover without ack answered %d, want 200 with the setup step re-rendered", rec.Code)
	}
}
