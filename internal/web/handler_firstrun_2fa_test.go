package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// beginFirstRunWith2FA submits step 1 with the box ticked and returns the body
// of the step-2 page plus the cookies that carry the pending id.
func beginFirstRunWith2FA(t *testing.T, s *Server) (string, []*http.Cookie) {
	t.Helper()
	rec := doFormRequest(s, "POST", "/firstrun",
		"username=admin&password=firstrunpassword1&password_confirm=firstrunpassword1"+
			"&ssh_port=22&ipv6_mode=filter&want_totp=1")
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

	body, _ := beginFirstRunWith2FA(t, s)

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

	_, cookies := beginFirstRunWith2FA(t, s)
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

	_, cookies := beginFirstRunWith2FA(t, s)
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

// THE test of this change. A board with a dead RTC must still end up with an
// account. If this fails, an optional feature has become a way of bricking the
// wizard.
func TestFirstRun2FA_SkipCreatesTheAccountWithoutAFactor(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	_, cookies := beginFirstRunWith2FA(t, s)

	rec := doFormRequest(s, "POST", "/firstrun/skip", "", cookies...)
	assertRedirect(t, rec, "/login")

	if s.cfg.IsFirstRun() {
		t.Fatal("skipping the second factor did not create the account — a wrong " +
			"clock can now prevent an account existing at all")
	}
	if s.cfg.TOTPEnabled() {
		t.Error("skipping the second factor enrolled one anyway")
	}
	if u, _ := s.cfg.Credentials(); u != "admin" {
		t.Errorf("the account was created as %q, so the wizard's answers were lost", u)
	}
}

// The wizard collects more than the account. Confirming a factor must not drop
// the ports and the IPv6 mode the operator chose above the password.
func TestFirstRun2FA_ConfirmStillStagesTheOtherAnswers(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	var savedTCP *shared.Command
	fc.OnCommand(shared.CmdSaveRules, func(c shared.Command) { savedTCP = &c })

	_, cookies := beginFirstRunWith2FA(t, s)
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

// A code that is right against a clock that is not.
func TestFirstRun2FA_AFarOutCodeDiagnosesTheClockAndStoresNothing(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	_, cookies := beginFirstRunWith2FA(t, s)
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

	_, cookies := beginFirstRunWith2FA(t, s)
	_ = doFormRequest(s, "POST", "/firstrun/confirm", "code=000000", cookies...)

	if !s.cfg.IsFirstRun() {
		t.Error("a wrong code created the account")
	}
}

// Without a pending id the routes create nothing and send the operator back.
func TestFirstRun2FA_NoPendingSetupStartsAgain(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	for _, path := range []string{"/firstrun/confirm", "/firstrun/skip"} {
		rec := doFormRequest(s, "POST", path, "code=000000")
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

	for _, path := range []string{"/firstrun/confirm", "/firstrun/skip"} {
		rec := doFormRequest(s, "POST", path, "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s answered %d on a configured install, want 404 — a "+
				"credential-writing route must not outlive the wizard", path, rec.Code)
		}
	}
}

// Unticked, nothing about the wizard changes.
func TestFirstRun2FA_UntickedIsTodaysPathExactly(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/firstrun",
		"username=admin&password=firstrunpassword1&password_confirm=firstrunpassword1"+
			"&ssh_port=22&ipv6_mode=filter")
	assertRedirect(t, rec, "/login")

	if s.cfg.IsFirstRun() {
		t.Fatal("the plain wizard stopped creating the account")
	}
	if s.cfg.TOTPEnabled() {
		t.Error("a wizard run with the box unticked enrolled a factor")
	}
}
