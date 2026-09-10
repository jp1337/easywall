package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// completeWizard drives the whole wizard: step 1 with form, then a valid TOTP
// code against the secret step 1 just generated. completeFirstRun's only
// caller is handleFirstRunConfirm now, so this is the only way any of these
// tests can reach a written account.
func completeWizard(t *testing.T, s *Server, form string) *httptest.ResponseRecorder {
	t.Helper()
	step1 := doFormRequest(s, "POST", "/firstrun", form)
	if step1.Code != http.StatusOK {
		t.Fatalf("step 1 answered %d, want 200 with the setup step rendered", step1.Code)
	}
	cookies := step1.Result().Cookies()
	raw, err := decodeTOTPSecret(firstRunPendingSecret(t, s, cookies))
	if err != nil {
		t.Fatal(err)
	}
	return doFormRequest(s, "POST", "/firstrun/confirm",
		"code="+totpAt(raw, stepAt(time.Now())), cookies...)
}

func TestHandleFirstRunGET_ShowsPage(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	rec := doRequest(s, "GET", "/firstrun", nil)
	assertStatus(t, rec, http.StatusOK)
}

// The /firstrun route is only registered when IsFirstRun() is true.
// When not first run, the route doesn't exist → 404 at router level.
// We test the handler directly to cover the redirect-to-login path.
func TestHandleFirstRunGET_Direct_RedirectsWhenNotFirstRun(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	req := httptest.NewRequest("GET", "/firstrun", nil)
	rec := httptest.NewRecorder()
	s.handleFirstRunGET(rec, req)
	assertRedirect(t, rec, "/login")
}

func TestHandleFirstRunPOST_Direct_RedirectsWhenNotFirstRun(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	req := httptest.NewRequest("POST", "/firstrun", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleFirstRunPOST(rec, req)
	assertRedirect(t, rec, "/login")
}

func TestHandleFirstRunPOST_EmptyUsername(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/firstrun", "username=&password=ValidPassword123!&password_confirm=ValidPassword123!")
	assertRedirect(t, rec, "/firstrun")
}

func TestHandleFirstRunPOST_PasswordTooShort(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/firstrun", "username=admin&password=short&password_confirm=short")
	assertRedirect(t, rec, "/firstrun")
}

func TestHandleFirstRunPOST_PasswordMismatch(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/firstrun", "username=admin&password=ValidPassword123!&password_confirm=DifferentPassword!")
	assertRedirect(t, rec, "/firstrun")
}

// A valid step 1 no longer creates the account: it moves the wizard to the
// TOTP step. See TestFirstRun2FA_StepOneStoresNothing and
// TestHandleFirstRunPOST_SavesCredentials for what happens on each side of
// that step.
func TestHandleFirstRunPOST_ValidSubmission(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/firstrun", "username=admin&password=ValidPassword123456!&password_confirm=ValidPassword123456!")
	assertStatus(t, rec, http.StatusOK)

	if !s.cfg.IsFirstRun() {
		t.Error("a valid step 1 submission created the account before a factor was confirmed")
	}
}

func TestHandleFirstRunPOST_SavesCredentials(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	if !s.cfg.IsFirstRun() {
		t.Fatal("expected first-run mode")
	}

	rec := completeWizard(t, s, "username=myadmin&password=MySecurePassword123!&password_confirm=MySecurePassword123!")
	assertStatus(t, rec, http.StatusOK)

	if s.cfg.Username != "myadmin" {
		t.Errorf("expected username 'myadmin', got %q", s.cfg.Username)
	}
	if s.cfg.Password == "" {
		t.Error("expected password hash to be set")
	}
	if s.cfg.IsFirstRun() {
		t.Error("expected first-run to be complete after successful setup")
	}
}

// The stated minimum is 12 characters, so 12 has to be accepted. This test used
// to log a note wondering whether it was, and assert nothing — leaving the
// boundary of the only password rule easywall has undefined.
func TestHandleFirstRunPOST_PasswordExactly12CharsIsAccepted(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	const pw = "exactly12ch!" // 12 characters, with a digit and a symbol
	if len(pw) != 12 {
		t.Fatalf("the test password is %d characters, not 12", len(pw))
	}

	rec := completeWizard(t, s, "username=admin&password="+pw+"&password_confirm="+pw)
	assertStatus(t, rec, http.StatusOK)

	if s.cfg.IsFirstRun() {
		t.Error("credentials were not saved, so the account was not created")
	}
	if !VerifyPassword(pw, s.cfg.Password) {
		t.Error("the stored hash does not verify against the password that was set")
	}
}

func TestHandleFirstRunPOST_Password11Chars(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	const pw = "eleven1234!" // 11 characters
	if len(pw) != 11 {
		t.Fatalf("the test password is %d characters, not 11", len(pw))
	}

	rec := doFormRequest(s, "POST", "/firstrun",
		"username=admin&password="+pw+"&password_confirm="+pw)
	assertRedirect(t, rec, "/firstrun")

	if !s.cfg.IsFirstRun() {
		t.Error("a rejected password must not create the account")
	}
}

// Step 1 no longer writes anything — the one write happens at confirm, once a
// code is checked — so a disk that cannot be written to does not stop the
// wizard from reaching the TOTP step. What happens when the disk really is
// touched and fails is TestFirstRun2FA_ConfirmSurvivesAFailedWrite's case.
func TestHandleFirstRunPOST_Step1DoesNotTouchDisk(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)
	s.cfg.configPath = "/nonexistent/path/web.toml"

	rec := doFormRequest(s, "POST", "/firstrun", "username=admin&password=ValidPassword123456!&password_confirm=ValidPassword123456!")
	assertStatus(t, rec, http.StatusOK)

	if !s.cfg.IsFirstRun() {
		t.Error("step 1 must not create the account by itself")
	}
}

// The wizard is the one moment an operator is already making decisions, so it
// asks the questions that matter on a fresh host: which port SSH is on, what
// happens to IPv6, and whether this installation may be counted.
func TestHandleFirstRunPOST_StagesTheChoices(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{
		IPv6:   shared.IPv6Config{Mode: shared.IPv6Filter},
		Docker: shared.DockerConfig{Enabled: true, AllowBridgeNetworks: true},
	}))

	rec := completeWizard(t, s,
		"username=admin&password=averysecurepass1!&password_confirm=averysecurepass1!"+
			"&ssh_port=2222&open_web=on&ipv6_mode=block&telemetry=on")
	assertStatus(t, rec, http.StatusOK)

	if s.cfg.IsFirstRun() {
		t.Fatal("the account was not created")
	}
	if !s.cfg.TelemetryEnabled() {
		t.Error("the operator agreed to be counted and that was not recorded")
	}

	// The last command the core saw is the settings save; the port save came
	// before it. Both have to have happened.
	last := fc.LastCommand()
	if last == nil || last.Type != shared.CmdSaveSettings {
		t.Fatalf("expected the IPv6 mode to be saved last, got %v", last)
	}
	var saved shared.NetworkSettings
	if err := json.Unmarshal(last.Payload, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.IPv6.Mode != shared.IPv6Block {
		t.Errorf("expected the chosen IPv6 mode, got %q", saved.IPv6.Mode)
	}
	// Read before write: answering one question must not reset the rest.
	if !saved.Docker.Enabled || !saved.Docker.AllowBridgeNetworks {
		t.Error("the Docker settings were overwritten by the wizard")
	}
}

// Telemetry is consent: unticked means no, and no is what gets stored.
func TestHandleFirstRunPOST_TelemetryIsOffUnlessTicked(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))

	completeWizard(t, s, "username=admin&password=averysecurepass1!&password_confirm=averysecurepass1!")

	if s.cfg.TelemetryEnabled() {
		t.Error("consent must never be assumed")
	}
	if s.cfg.Telemetry == nil {
		t.Error("the answer should be recorded, not left unset")
	}
}

// The SSH port is the one answer that can lock the operator out, so it is
// checked while the page is still in front of them — before the account exists
// and the wizard closes.
func TestHandleFirstRunPOST_RejectsAnImpossibleSSHPortBeforeCreatingTheAccount(t *testing.T) {
	for _, port := range []string{"0", "70000", "22abc", "-1"} {
		fc := newFakeCore(t)
		s := newFirstRunTestServer(t, fc)

		rec := doFormRequest(s, "POST", "/firstrun",
			"username=admin&password=averysecurepass1!&password_confirm=averysecurepass1!&ssh_port="+port)
		assertRedirect(t, rec, "/firstrun")

		if !s.cfg.IsFirstRun() {
			t.Errorf("port %q: the account must not be created while the form is wrong", port)
		}
	}
}

// An empty port field means the default, not a refusal.
func TestHandleFirstRunPOST_EmptySSHPortMeans22(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))

	var saved []shared.PortRule
	fc.OnCommand(shared.CmdSaveRules, func(cmd shared.Command) {
		var p shared.SaveRulesPayload
		if err := json.Unmarshal(cmd.Payload, &p); err != nil {
			return
		}
		raw, _ := json.Marshal(p.Rules)
		_ = json.Unmarshal(raw, &saved)
	})

	completeWizard(t, s, "username=admin&password=averysecurepass1!&password_confirm=averysecurepass1!&ssh_port=")

	if len(saved) == 0 || saved[0].Port != "22" {
		t.Fatalf("expected port 22 staged first, got %+v", saved)
	}
	if !saved[0].SSH {
		t.Error("the SSH port should be staged with brute-force protection")
	}
}

// The input policy is drop and nothing opens the web port by itself, so a
// wizard that stages only SSH hands the operator a rule set whose first apply
// cuts off the page they applied it from. The acceptance window then rolls the
// whole apply back, and nothing anywhere says why.
func TestHandleFirstRunPOST_StagesThePortThisInterfaceIsServedOn(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))

	var saved []shared.PortRule
	fc.OnCommand(shared.CmdSaveRules, func(cmd shared.Command) {
		var p shared.SaveRulesPayload
		if err := json.Unmarshal(cmd.Payload, &p); err != nil {
			return
		}
		raw, _ := json.Marshal(p.Rules)
		_ = json.Unmarshal(raw, &saved)
	})

	completeWizard(t, s, "username=admin&password=averysecurepass1!&password_confirm=averysecurepass1!&ssh_port=22")

	_, want, err := net.SplitHostPort(s.cfg.BindAddr)
	if err != nil {
		t.Fatalf("test server has an unreadable bind_addr %q", s.cfg.BindAddr)
	}
	found := false
	for _, r := range saved {
		if r.Port == want {
			found = true
			if r.SSH {
				t.Error("the web port must not be routed through the SSH brute-force chain")
			}
		}
	}
	if !found {
		t.Errorf("port %s (this interface) was not staged; got %+v", want, saved)
	}
}

// Everything the operator answered has to survive a rejected submission. The
// SSH port is the one that matters: reverting it to 22 unnoticed stages a port
// their machine does not listen on, and the passwords are the one thing that
// must not come back.
func TestHandleFirstRunPOST_RejectedSubmissionKeepsTheAnswers(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/firstrun",
		"username=operator&password=averysecurepass1!&password_confirm=mismatch"+
			"&ssh_port=2222&open_web=on&ipv6_mode=block&telemetry=on")

	back := doRequest(s, "GET", "/firstrun", nil, rec.Result().Cookies()...)
	body := back.Body.String()
	for _, want := range []string{`value="operator"`, `value="2222"`, `name="open_web" class="toggle" checked`} {
		if !strings.Contains(body, want) {
			t.Errorf("the re-rendered wizard lost %s", want)
		}
	}
	if !strings.Contains(body, `value="block" class="radio" checked`) {
		t.Error("the re-rendered wizard lost the IPv6 choice")
	}
	if strings.Contains(body, "averysecurepass1!") {
		t.Error("the password came back in the page")
	}
}

// The core being unreachable must not cost the operator their account — without
// one they cannot get in at all, and the wizard closes either way.
func TestHandleFirstRunPOST_CreatesTheAccountEvenIfTheCoreIsDown(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)
	fc.SetDefaultResponse(errorRespFor("core unavailable"))

	rec := completeWizard(t, s, "username=admin&password=averysecurepass1!&password_confirm=averysecurepass1!")
	assertStatus(t, rec, http.StatusOK)

	if s.cfg.IsFirstRun() {
		t.Error("the account must be created even when the choices cannot be staged")
	}
	if !VerifyPassword("averysecurepass1!", s.cfg.Password) {
		t.Error("the stored hash does not verify")
	}
}

// Nothing the wizard sets may reach the kernel on its own: easywall's model is
// that rules go live through a deliberate apply with a window to undo it, and
// the first run is the worst moment to make an exception.
func TestHandleFirstRunPOST_StagesButNeverApplies(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))

	var applied bool
	fc.OnCommand(shared.CmdApplyRules, func(shared.Command) { applied = true })

	completeWizard(t, s, "username=admin&password=averysecurepass1!&password_confirm=averysecurepass1!&ssh_port=22")

	if applied {
		t.Error("the wizard must not apply rules")
	}
}

// Two setups arriving together both passed the handler's IsFirstRun check and
// both wrote, so the second one decided who owns the firewall. The check now
// sits under the same lock as the write.
func TestSaveFirstRun_SecondSetupCannotTakeOverTheAccount(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))

	// The hash is computed once, outside the goroutines, and that is not a
	// tidy-up: this is what killed Test (ubuntu-24.04) with exit 143. argon2id
	// is configured at m=64 MiB (defaultArgon2Params), so 50 concurrent
	// HashPassword calls hold 3.2 GB of live heap at the same instant, and the
	// race detector shadows every byte of it — measured, this one test took the
	// binary's peak RSS from 0.6 GB to 9.7 GB and the run peaked at 16.6 GB
	// against a runner with 16 GB. Nothing here is testing the KDF. Hoisting it
	// also makes the probe sharper, because the goroutines now converge on
	// SaveFirstRun instead of arriving whenever their own hash finished.
	hash, err := HashPassword("averysecurepass1!")
	if err != nil {
		t.Fatal(err)
	}

	const attempts = 50
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = s.cfg.SaveFirstRun(FirstRunAccount{
				Username:     fmt.Sprintf("operator%d", i),
				PasswordHash: hash,
				Telemetry:    false,
			})
		}(i)
	}
	wg.Wait()

	user, _ := s.cfg.Credentials()
	if user == "" {
		t.Fatal("no account was created at all")
	}

	// Whoever won, nobody may replace them afterwards.
	if err := s.cfg.SaveFirstRun(FirstRunAccount{
		Username:     "intruder",
		PasswordHash: hash,
		Telemetry:    true,
	}); !errors.Is(err, ErrAlreadySetUp) {
		t.Fatalf("a later setup was accepted: %v", err)
	}
	if after, _ := s.cfg.Credentials(); after != user {
		t.Errorf("the account changed hands: %q became %q", user, after)
	}
}
