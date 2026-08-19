package web

import (
	"os"
	"strings"
	"testing"
)

// The comments are the point. config/web.toml is three kilobytes of them, the
// configuration page still sends operators to that file, and the encoder
// serialises a struct — so a save that falls back to it replaces all of it with
// bare lines. mergeConfig has to reach the two new keys, or they are simply not
// written and the factor silently stops existing on the next password change.
func TestConfig_TOTPKeysSurviveTheSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/web.toml"
	original := `# easywall web configuration
# A comment that has to survive.

session_key = "test-session-key-32bytes-padding!"
username    = "admin"
password    = ""

# ─── Second factor ─────────────────────────────────────────────────────────
# Written by the interface. Delete both lines to switch the factor off.
totp_secret    = ""
recovery_codes = []

[tls]
cert = ""
key  = ""
`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTOTP("JBSWY3DPEHPK3PXP", []string{"$argon2id$hash1", "$argon2id$hash2"}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)

	for _, want := range []string{
		"A comment that has to survive.",
		"# ─── Second factor",
		"Delete both lines to switch the factor off.",
		`totp_secret    = "JBSWY3DPEHPK3PXP"`,
		`recovery_codes = ["$argon2id$hash1", "$argon2id$hash2"]`,
		"[tls]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the saved file does not contain %q\n--- file ---\n%s", want, got)
		}
	}

	// And it must load back as what was asked for.
	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("the file this program just wrote does not load: %v", err)
	}
	if reloaded.TOTPSecret() != "JBSWY3DPEHPK3PXP" {
		t.Errorf("secret came back as %q", reloaded.TOTPSecret())
	}
	if len(reloaded.RecoveryCodes()) != 2 {
		t.Errorf("recovery codes came back as %v", reloaded.RecoveryCodes())
	}
}

// A file written before 2.8 has neither key. It must load, and the first save
// must add both somewhere a reader expects them — above the first table header,
// or the value lands inside [tls] and means something else entirely.
func TestConfig_AFileWithoutTheNewKeysLoadsAndGainsThem(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/web.toml"
	if err := os.WriteFile(path, []byte(`
session_key = "test-session-key-32bytes-padding!"
username    = "admin"
password    = ""

[tls]
cert = ""
key  = ""
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("a pre-2.8 web.toml does not load: %v", err)
	}
	if cfg.TOTPEnabled() {
		t.Error("a config with no totp_secret reports a factor as enabled")
	}
	if err := cfg.SaveTOTP("JBSWY3DPEHPK3PXP", []string{"$argon2id$hash1"}); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(path)
	got := string(raw)
	secretAt := strings.Index(got, "totp_secret")
	codesAt := strings.Index(got, "recovery_codes")
	tableAt := strings.Index(got, "[tls]")
	if secretAt < 0 || codesAt < 0 {
		t.Fatalf("the new keys were not written at all\n--- file ---\n%s", got)
	}
	if secretAt > tableAt || codesAt > tableAt {
		t.Errorf("a new key landed inside [tls], where it is a different key\n--- file ---\n%s", got)
	}
}

// Switching the factor off has to clear both, and clearing means writing the
// empty forms rather than leaving the previous values in the file. A secret that
// survives "turn it off" is a factor that is still enforced after the interface
// says it is not.
func TestConfig_DisablingClearsBothKeys(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/web.toml"
	_ = os.WriteFile(path, []byte(`
session_key    = "test-session-key-32bytes-padding!"
username       = "admin"
password       = ""
totp_secret    = "JBSWY3DPEHPK3PXP"
recovery_codes = ["$argon2id$hash1"]
`), 0600)

	cfg, _ := LoadConfig(path)
	if !cfg.TOTPEnabled() {
		t.Fatal("a config with a secret does not report the factor as enabled")
	}
	if err := cfg.SaveTOTP("", nil); err != nil {
		t.Fatal(err)
	}

	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.TOTPSecret() != "" {
		t.Errorf("the secret survived being switched off: %q", reloaded.TOTPSecret())
	}
	if len(reloaded.RecoveryCodes()) != 0 {
		t.Errorf("the recovery codes survived being switched off: %v", reloaded.RecoveryCodes())
	}
}

// Enabling or disabling a factor ends every other session at that moment. A
// second factor that lets previously open sessions run on protects from the next
// login, not from now.
func TestCredentialFingerprint_CoversTheTOTPState(t *testing.T) {
	hash := "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA"

	off := credentialFingerprint(hash, "")
	on := credentialFingerprint(hash, "JBSWY3DPEHPK3PXP")
	other := credentialFingerprint(hash, "KRSXG5BAMFXG65DI")

	if off == on {
		t.Error("enabling a second factor did not change the fingerprint, so no other session ended")
	}
	if on == other {
		t.Error("two different secrets produce the same fingerprint")
	}
	if credentialFingerprint(hash, "") != off {
		t.Error("the fingerprint is not stable for the same inputs")
	}
	// And it must not be derivable back to either input.
	if strings.Contains(off, hash) || strings.Contains(on, "JBSWY3DPEHPK3PXP") {
		t.Error("the fingerprint carries its input")
	}
}
