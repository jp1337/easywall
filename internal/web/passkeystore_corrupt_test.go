package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-webauthn/webauthn/webauthn"
)

// corruptTheStore writes a passkeys.json that will not parse and rebuilds the
// server's store from it, which is what a restart on a host with a truncated
// backup, a half-restored data_dir or a bad disk actually leaves behind. The
// fixture's passkeyCount closure reads s.passkeys at call time, so replacing
// the pointer is enough.
func corruptTheStore(t *testing.T, s *Server) {
	t.Helper()
	if err := os.WriteFile(s.passkeys.path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	s.passkeys = newPasskeyStore(s.passkeys.path)
	if !s.passkeys.isCorrupt() {
		t.Fatal("a file that does not parse was not recorded as corrupt")
	}
}

// loginWithPassword performs the password step and returns the response.
func loginWithPassword(s *Server) *httptest.ResponseRecorder {
	return doFormRequest(s, "POST", "/login", "username=admin&password=testpassword123%21")
}

// asBrowserWould keeps the last cookie of each name, which is what a cookie jar
// holds after a response that set one name twice. The recovery path sets the
// flash before it grants the session, so the raw recorder hands back two
// easywall_session cookies and a request carrying both is read as the first —
// the flash-only one, with nobody signed in. A browser never sees that.
func asBrowserWould(cookies []*http.Cookie) []*http.Cookie {
	last := map[string]*http.Cookie{}
	order := []string{}
	for _, c := range cookies {
		if _, seen := last[c.Name]; !seen {
			order = append(order, c.Name)
		}
		last[c.Name] = c
	}
	out := make([]*http.Cookie, 0, len(order))
	for _, name := range order {
		out = append(out, last[name])
	}
	return out
}

// TestAnUnreadablePasskeyStoreStillDemandsASecondFactor is the defect itself:
// an account whose only factor is a passkey, and a passkeys.json that will not
// parse, used to sign in on the password alone.
func TestAnUnreadablePasskeyStoreStillDemandsASecondFactor(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))
	hash, _ := HashPassword(testPassword)
	s.cfg.Password = hash
	corruptTheStore(t, s)

	rec := loginWithPassword(s)
	assertRedirect(t, rec, "/login/verify")

	// And what it handed out authenticates nothing.
	got := doRequest(s, "GET", "/dashboard", nil, rec.Result().Cookies()...)
	assertRedirect(t, got, "/login")
}

// TestARecoveryCodeSignsInPastAnUnreadableStore is the other half, and the one
// that makes the first half safe: the way back in does not live in the file
// that broke. Eight codes are minted at the first factor whichever factor it
// is, so a passkey-only account has them.
func TestARecoveryCodeSignsInPastAnUnreadableStore(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))
	hash, _ := HashPassword(testPassword)
	s.cfg.Password = hash

	codes, hashes, err := newRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.cfg.SaveRecoveryCodes(hashes); err != nil {
		t.Fatal(err)
	}
	if s.cfg.TOTPSecret() != "" {
		t.Fatal("this fixture is meant to have no TOTP secret at all")
	}
	corruptTheStore(t, s)

	first := loginWithPassword(s)
	assertRedirect(t, first, "/login/verify")

	rec := doFormRequest(s, "POST", "/login/verify", "code="+codes[0], first.Result().Cookies()...)
	assertRedirect(t, rec, "/dashboard")
	assertStatus(t, doRequest(s, "GET", "/dashboard", nil, asBrowserWould(rec.Result().Cookies())...), http.StatusOK)

	if n := len(s.cfg.RecoveryCodes()); n != recoveryCodeCount-1 {
		t.Errorf("%d codes left, want %d", n, recoveryCodeCount-1)
	}
}

// TestNoPasskeyFileAtAllIsUnchanged is the ordinary state of every install that
// has never enrolled one, and it must not have grown a gate.
func TestNoPasskeyFileAtAllIsUnchanged(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))
	hash, _ := HashPassword(testPassword)
	s.cfg.Password = hash

	if _, err := os.Stat(s.passkeys.path); !os.IsNotExist(err) {
		t.Fatalf("the fixture already has a passkey store: %v", err)
	}
	if s.passkeys.isCorrupt() {
		t.Fatal("an absent file was treated as unreadable; every fresh install is now gated")
	}
	if n := s.factorCount(); n != 0 {
		t.Fatalf("factorCount() = %d with no factors at all, want 0", n)
	}
	assertRedirect(t, loginWithPassword(s), "/dashboard")
}

// TestAnUnreadableStoreDoesNotLicenseRemovingTheWorkingFactor: TOTP plus a file
// nobody can read is two by the count and one in reality, and switching the
// real one off would leave an account behind a factor that cannot be presented.
func TestAnUnreadableStoreDoesNotLicenseRemovingTheWorkingFactor(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))
	if err := s.cfg.SaveTOTP("JBSWY3DPEHPK3PXP", nil); err != nil {
		t.Fatal(err)
	}
	corruptTheStore(t, s)

	if n := s.factorCount(); n != 2 {
		t.Fatalf("factorCount() = %d, want 2 (TOTP plus the unknown file)", n)
	}
	if s.mayRemoveFactor() {
		t.Error("mayRemoveFactor() allowed the only presentable factor to be switched off")
	}

	// Wired to the route, not merely present in the package.
	resp := s.postAuthed(t, "/password/2fa/disable", map[string]string{"current_password": testPassword})
	defer resp.Body.Close()
	if got := s.cfg.TOTPSecret(); got == "" {
		t.Fatal("TOTP was switched off while the passkey store was unreadable")
	}
}

// TestTheDemoIsNotGatedByAnUnreadableStore: the demo account belongs to nobody
// and every control on it must stay pressable.
func TestTheDemoIsNotGatedByAnUnreadableStore(t *testing.T) {
	s := newDemoTestServer(t)
	corruptTheStore(t, s)
	if !s.mayRemoveFactor() {
		t.Error("the demo was gated by an unreadable passkey store")
	}
}

// TestAFileThatCannotBeReadCountsToo — not only unparseable contents. A
// directory where the file should be is the shape a mangled data_dir actually
// takes, and os.ReadFile fails before any JSON is involved.
func TestAFileThatCannotBeReadCountsToo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "passkeys.json")
	if err := os.Mkdir(path, 0750); err != nil {
		t.Fatal(err)
	}
	if !newPasskeyStore(path).isCorrupt() {
		t.Error("a passkey store that cannot be read at all was treated as an empty one")
	}
}

// TestAnUnreadableStoreIsMovedAsideNotOverwritten: the next enrolment used to
// write straight over the file, destroying whatever credentials were in it
// along with any chance of repairing it by hand.
func TestAnUnreadableStoreIsMovedAsideNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "passkeys.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}

	p := newPasskeyStore(path)
	if err := p.add("a new key", webauthn.Credential{ID: []byte("cred-1")}); err != nil {
		t.Fatalf("add: %v", err)
	}

	kept, err := os.ReadFile(path + ".corrupt")
	if err != nil {
		t.Fatalf("the unreadable file was not kept: %v", err)
	}
	if string(kept) != "{not json" {
		t.Errorf("the kept file is not the original: %q", kept)
	}
	// And the phantom factor does not outlive it: the enrolled set is known
	// again, and it is the one real passkey just written.
	if p.isCorrupt() {
		t.Error("the store still counts as unknown after the bad file was moved aside")
	}
	if n := len(newPasskeyStore(path).all()); n != 1 {
		t.Errorf("the rewritten store holds %d passkeys, want 1", n)
	}
}
