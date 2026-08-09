package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

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

func TestHandleFirstRunPOST_ValidSubmission(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/firstrun", "username=admin&password=ValidPassword123456!&password_confirm=ValidPassword123456!")
	assertRedirect(t, rec, "/login")
}

func TestHandleFirstRunPOST_SavesCredentials(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	if !s.cfg.IsFirstRun() {
		t.Fatal("expected first-run mode")
	}

	rec := doFormRequest(s, "POST", "/firstrun", "username=myadmin&password=MySecurePassword123!&password_confirm=MySecurePassword123!")
	assertRedirect(t, rec, "/login")

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

	const pw = "exactly12chr" // 12 characters
	if len(pw) != 12 {
		t.Fatalf("the test password is %d characters, not 12", len(pw))
	}

	rec := doFormRequest(s, "POST", "/firstrun",
		"username=admin&password="+pw+"&password_confirm="+pw)
	assertRedirect(t, rec, "/login")

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

func TestHandleFirstRunPOST_SaveCredentialsError(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	// Make SaveCredentials fail by pointing configPath to an invalid location
	s.cfg.configPath = "/nonexistent/path/web.toml"

	rec := doFormRequest(s, "POST", "/firstrun", "username=admin&password=ValidPassword123456!&password_confirm=ValidPassword123456!")
	// Should redirect back to /firstrun (save_error flash)
	assertRedirect(t, rec, "/firstrun")
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

	rec := doFormRequest(s, "POST", "/firstrun",
		"username=admin&password=averysecurepass1&password_confirm=averysecurepass1"+
			"&ssh_port=2222&open_web=on&ipv6_mode=block&telemetry=on")
	assertRedirect(t, rec, "/login")

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

	doFormRequest(s, "POST", "/firstrun",
		"username=admin&password=averysecurepass1&password_confirm=averysecurepass1")

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
			"username=admin&password=averysecurepass1&password_confirm=averysecurepass1&ssh_port="+port)
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

	doFormRequest(s, "POST", "/firstrun",
		"username=admin&password=averysecurepass1&password_confirm=averysecurepass1&ssh_port=")

	if len(saved) != 1 || saved[0].Port != "22" {
		t.Fatalf("expected port 22 staged, got %+v", saved)
	}
	if !saved[0].SSH {
		t.Error("the SSH port should be staged with brute-force protection")
	}
}

// The core being unreachable must not cost the operator their account — without
// one they cannot get in at all, and the wizard closes either way.
func TestHandleFirstRunPOST_CreatesTheAccountEvenIfTheCoreIsDown(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)
	fc.SetDefaultResponse(errorRespFor("core unavailable"))

	rec := doFormRequest(s, "POST", "/firstrun",
		"username=admin&password=averysecurepass1&password_confirm=averysecurepass1")
	assertRedirect(t, rec, "/login")

	if s.cfg.IsFirstRun() {
		t.Error("the account must be created even when the choices cannot be staged")
	}
	if !VerifyPassword("averysecurepass1", s.cfg.Password) {
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

	doFormRequest(s, "POST", "/firstrun",
		"username=admin&password=averysecurepass1&password_confirm=averysecurepass1&ssh_port=22")

	if applied {
		t.Error("the wizard must not apply rules")
	}
}
