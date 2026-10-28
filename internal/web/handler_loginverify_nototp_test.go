package web

import (
	"bytes"
	"log/slog"
	"net/url"
	"strings"
	"testing"

	"github.com/go-webauthn/webauthn/webauthn"
)

// TestASixDigitCodeOnAPasskeyOnlyAccountIsJustWrong: an account whose only
// factor is a passkey has no TOTP secret, and the code field is still on the
// page. acceptTOTP decoded the empty secret, failed, and logged at ERROR that
// the stored secret was unusable — advising the operator to delete
// passkeys.json, the one factor the account has. Nothing was broken; the code
// was wrong.
func TestASixDigitCodeOnAPasskeyOnlyAccountIsJustWrong(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))
	if err := s.passkeys.add("a key", webauthn.Credential{ID: []byte("cred-1")}); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	pending := pendingCookieOf(t, doFormRequest(s, "POST", "/login",
		"username=admin&password="+url.QueryEscape(testPassword)))
	rec := doFormRequest(s, "POST", "/login/verify", "code=123456", pending)
	assertRedirect(t, rec, "/login/verify")

	if out := logs.String(); strings.Contains(out, "level=ERROR") || strings.Contains(out, "passkeys.json") {
		t.Errorf("a wrong code on a passkey-only account logged:\n%s", out)
	}
}
