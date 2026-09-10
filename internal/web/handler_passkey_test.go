package web

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"
)

// passkeyTestOption configures a Server built by newPasskeyTestServer, in the
// same functional-option shape the rest of Go's ecosystem uses for optional
// constructor arguments — there is no natural zero value for "a hostname" or
// "a TOTP secret" that would not also read as a deliberate choice of one.
type passkeyTestOption func(*Server)

// withHostname sets tls.hostname directly on the fixture's config, which is
// what (*Config).Hostname reads.
//
// A non-empty host also marks a certificate as operator-supplied
// (s.cfg.TLS.CertFile), so passkeyUnavailableReason's self-signed check does
// not block the ceremony tests this option exists for — they are about the
// ceremony, not about that check, which has its own dedicated test
// (TestPasskeysAreRefusedOnASelfSignedCertificate) built without this helper.
// The empty-hostname fixture is unaffected: the hostname check refuses first
// regardless of the certificate.
func withHostname(host string) passkeyTestOption {
	return func(s *Server) {
		s.cfg.TLS.Hostname = host
		if host != "" {
			s.cfg.TLS.CertFile = "test-fixture-cert.pem"
		}
	}
}

// withTOTP enrols a TOTP secret the way SaveTOTP itself does, for the tests
// that need a second factor already in place — TestRemovingAPasskeyEndsSessions
// removes the *only* passkey and needs another factor for mayRemoveFactor to
// allow that at all.
func withTOTP(secret string) passkeyTestOption {
	return func(s *Server) {
		if err := s.cfg.SaveTOTP(secret, nil); err != nil {
			panic(err)
		}
	}
}

// withDemo swaps in the demo client the way newDemoTestServer does, applied
// after the Server already exists rather than duplicating its construction.
func withDemo() passkeyTestOption {
	return func(s *Server) {
		s.cfg.DemoMode = true
		s.client = NewDemoClient()
		close(s.eventsStop)
		s.events = newAuditEvents(s.client, true)
		s.eventsStop = make(chan struct{})
		go s.events.run(s.eventsStop)
		s.router = s.buildRouter(s.cfg)
	}
}

// newPasskeyTestServer builds a Server with a real, empty passkey store —
// unlike newFactorTestServer's fixed-count stand-in — and a bind address
// ending in :443, so publicOrigin() names no port and a fixture's
// RelyingParty.Origin can be written as a bare "https://host", the way an
// ordinary HTTPS installation's would read.
func newPasskeyTestServer(t *testing.T, opts ...passkeyTestOption) *Server {
	t.Helper()
	s := newTestServer(t, newFakeCore(t))
	s.cfg.BindAddr = "127.0.0.1:443"
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// readBody drains and returns a response body as a string.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}

// signIn performs a full login and returns the session cookie it sets.
// Returned as a pointer so a caller like enrolPasskeyWithCookie can update it
// in place after a step that re-stamps the session, the way a browser's own
// cookie jar would pick up the new value without the test having to thread a
// second variable through.
//
// When the fixture also carries a TOTP secret (TestRemovingAPasskeyEndsSessions
// enrols one so mayRemoveFactor allows taking the passkey away), the password
// step alone only opens the pending state; this completes the second step
// itself with a code valid against that secret, rather than making every
// caller decide whether a second factor is in play.
func (s *Server) signIn(t *testing.T) *http.Cookie {
	t.Helper()
	vals := url.Values{"username": {"admin"}, "password": {testPassword}}
	resp := doFormRequest(s, "POST", "/login", vals.Encode()).Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login answered %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
	for _, c := range resp.Cookies() {
		if c.Name == SessionName {
			return c
		}
	}

	var pending *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == pendingCookieName {
			pending = c
		}
	}
	if pending == nil {
		t.Fatal("login set neither a session cookie nor a pending one")
	}
	raw, err := decodeTOTPSecret(s.cfg.TOTPSecret())
	if err != nil {
		t.Fatalf("decode the fixture's own TOTP secret: %v", err)
	}
	code := totpAt(raw, stepAt(time.Now()))
	verify := doFormRequest(s, "POST", "/login/verify", url.Values{"code": {code}}.Encode(), pending).Result()
	if verify.StatusCode != http.StatusSeeOther {
		t.Fatalf("login/verify answered %d", verify.StatusCode)
	}
	for _, c := range verify.Cookies() {
		if c.Name == SessionName {
			return c
		}
	}
	t.Fatal("login/verify did not set a session cookie")
	return nil
}

// getWithCookie performs a GET carrying exactly the given cookie — no
// makeAuthCookie involved — for asserting whether a specific, already-issued
// session still works.
func (s *Server) getWithCookie(t *testing.T, path string, cookie *http.Cookie) *http.Response {
	t.Helper()
	return doRequest(s, "GET", path, nil, cookie).Result()
}

// enrolPasskeyWithCookie drives a full registration ceremony with a virtual
// authenticator, authenticated as the given session, and updates cookie in
// place with the value handlePasskeyFinish's restamp leaves — the same way a
// browser holding that cookie would pick up the new one, without the caller
// having to notice a re-stamp happened at all. Returns the enrolled
// credential's ID.
func enrolPasskeyWithCookie(t *testing.T, s *Server, name string, cookie *http.Cookie) []byte {
	t.Helper()

	begin := doRequest(s, "POST", "/password/passkey/begin", nil, cookie).Result()
	if begin.StatusCode != http.StatusOK {
		t.Fatalf("begin answered %d", begin.StatusCode)
	}
	options := readBody(t, begin)

	var challenge *http.Cookie
	for _, c := range begin.Cookies() {
		if c.Name == passkeyPendingCookieName {
			challenge = c
		}
	}
	if challenge == nil {
		t.Fatal("begin did not set the passkey challenge cookie")
	}

	parsed, err := virtualwebauthn.ParseAttestationOptions(options)
	if err != nil {
		t.Fatalf("parse the creation options: %v", err)
	}

	auth := virtualwebauthn.NewAuthenticator()
	rp := virtualwebauthn.RelyingParty{
		Name:   "easywall",
		ID:     s.cfg.Hostname(),
		Origin: s.publicOrigin(),
	}
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	attestation := virtualwebauthn.CreateAttestationResponse(rp, auth, cred, *parsed)

	vals := url.Values{}
	vals.Set("name", name)
	vals.Set("credential", attestation)
	finish := doFormRequest(s, "POST", "/password/passkey/finish", vals.Encode(), cookie, challenge).Result()
	if finish.StatusCode != http.StatusOK {
		t.Fatalf("finish answered %d: %s", finish.StatusCode, readBody(t, finish))
	}
	for _, c := range finish.Cookies() {
		if c.Name == SessionName {
			cookie.Value = c.Value
		}
	}
	return cred.ID
}

// enrolPasskey enrols against a fresh, unrelated session — for the tests that
// only care about the store and the config afterwards, not about a specific
// session surviving the ceremony.
func enrolPasskey(t *testing.T, s *Server, name string) []byte {
	t.Helper()
	return enrolPasskeyWithCookie(t, s, name, makeAuthCookie(t, s))
}

// TestPasskeysAreRefusedWithoutAHostname — WebAuthn's Relying Party ID must be
// a registrable domain, and a ceremony begun with an empty one fails in the
// browser with a SecurityError the operator cannot act on.
func TestPasskeysAreRefusedWithoutAHostname(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname(""))

	if _, err := s.webAuthn(); err == nil {
		t.Fatal("a WebAuthn instance was built with no Relying Party ID")
	}
	if got := s.passkeyUnavailableReason(); got != "passkey_no_hostname" {
		t.Errorf("reason = %q, want passkey_no_hostname — the operator needs to know which of the three things to change", got)
	}

	resp := s.postAuthed(t, "/password/passkey/begin", nil)
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Error("the registration ceremony started with no Relying Party ID")
	}
}

// TestPasskeysAreRefusedInDemoMode — the demo runs behind a real domain, so
// passkeys would otherwise work, and any visitor could leave one behind.
func TestPasskeysAreRefusedInDemoMode(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"), withDemo())

	if got := s.passkeyUnavailableReason(); got != "passkey_demo" {
		t.Errorf("reason = %q, want passkey_demo", got)
	}
	resp := s.postAuthed(t, "/password/passkey/begin", nil)
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Error("a registration ceremony started in the demo")
	}
}

// TestPasskeysAreRefusedOnASelfSignedCertificate — a hostname with neither
// ACME nor an operator-supplied certificate is the self-signed pair easywall
// generates itself, which no browser trusts by default. Built without
// withHostname's cert-marking side effect, so nothing here stands in for the
// check under test.
func TestPasskeysAreRefusedOnASelfSignedCertificate(t *testing.T) {
	s := newPasskeyTestServer(t)
	s.cfg.TLS.Hostname = "firewall.example.org" // no CertFile, ACME left off

	if got := s.passkeyUnavailableReason(); got != "passkey_self_signed" {
		t.Errorf("reason = %q, want passkey_self_signed", got)
	}

	resp := s.postAuthed(t, "/password/passkey/begin", nil)
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Error("the registration ceremony started on a self-signed certificate")
	}
}

// TestAPasskeyCanBeEnrolledAndCounts drives a real ceremony with a virtual
// authenticator and asserts the result satisfies the mandate.
func TestAPasskeyCanBeEnrolledAndCounts(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"))
	auth := virtualwebauthn.NewAuthenticator()
	rp := virtualwebauthn.RelyingParty{
		Name:   "easywall",
		ID:     "firewall.example.org",
		Origin: "https://firewall.example.org",
	}
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	begin := s.postAuthed(t, "/password/passkey/begin", nil)
	if begin.StatusCode != 200 {
		t.Fatalf("begin answered %d", begin.StatusCode)
	}
	options := readBody(t, begin)

	parsed, err := virtualwebauthn.ParseAttestationOptions(options)
	if err != nil {
		t.Fatalf("parse the creation options: %v", err)
	}
	attestation := virtualwebauthn.CreateAttestationResponse(rp, auth, cred, *parsed)

	finish := s.postAuthedJSON(t, "/password/passkey/finish",
		map[string]string{"name": "YubiKey on the keyring"}, attestation)
	if finish.StatusCode != 200 {
		t.Fatalf("finish answered %d: %s", finish.StatusCode, readBody(t, finish))
	}

	if n := s.factorCount(); n != 1 {
		t.Errorf("factorCount() = %d after enrolling one passkey, want 1", n)
	}
	if !s.hasSecondFactor() {
		t.Error("the gate is still closed after a passkey was enrolled")
	}

	// The stored record is the one this ceremony actually produced — the
	// credential id the authenticator generated, under the name submitted —
	// not merely a count. factorCount()==1 alone would pass just as well for
	// a record holding the wrong id or a name nobody typed.
	stored := s.passkeys.all()
	if len(stored) != 1 {
		t.Fatalf("%d passkeys stored, want 1", len(stored))
	}
	if stored[0].Name != "YubiKey on the keyring" {
		t.Errorf("stored name = %q, want %q", stored[0].Name, "YubiKey on the keyring")
	}
	if !bytes.Equal(stored[0].ID, cred.ID) {
		t.Errorf("stored credential id = %x, want the one the ceremony produced (%x)", stored[0].ID, cred.ID)
	}
}

// TestBeginRegistrationExcludesEnrolledCredentials.
//
// Without excludeCredentials, the same physical authenticator can be
// enrolled again and again under a different name: the operator's list shows
// several entries for one device, factorCount() reports several factors, and
// the mandate rests on a number that overstates how many independent things
// actually stand between an attacker and the account. mayRemoveFactor() is
// the sharper case — it would allow removing "one of two" when both entries
// are the same key.
//
// Asserts the excluded id itself, not merely that the list is non-empty: a
// list carrying the wrong id would satisfy a bare non-nil check while still
// leaving the real device excludable a second time.
func TestBeginRegistrationExcludesEnrolledCredentials(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"))
	id := enrolPasskey(t, s, "already enrolled")

	begin := s.postAuthed(t, "/password/passkey/begin", nil)
	defer begin.Body.Close()
	if begin.StatusCode != 200 {
		t.Fatalf("begin answered %d", begin.StatusCode)
	}
	parsed, err := virtualwebauthn.ParseAttestationOptions(readBody(t, begin))
	if err != nil {
		t.Fatalf("parse the creation options: %v", err)
	}

	want := base64.RawURLEncoding.EncodeToString(id)
	excluded := false
	for _, c := range parsed.ExcludeCredentials {
		if c == want {
			excluded = true
		}
	}
	if !excluded {
		t.Errorf("excludeCredentials = %v, want it to contain %q (the already-enrolled credential)",
			parsed.ExcludeCredentials, want)
	}
}

// TestPasskeyFinishRefusesAnEmptyName.
//
// app.js's field is required, and reportValidity() refuses to submit an
// empty one — but the server does not trust that: a request built without
// the browser's help (a script, a hand-edited form) must be refused on its
// own terms too. A whitespace-only value is the interesting case, not a
// truly empty one: TrimSpace is what makes " " indistinguishable from "".
func TestPasskeyFinishRefusesAnEmptyName(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"))
	auth := virtualwebauthn.NewAuthenticator()
	rp := virtualwebauthn.RelyingParty{
		Name:   "easywall",
		ID:     "firewall.example.org",
		Origin: "https://firewall.example.org",
	}
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	begin := s.postAuthed(t, "/password/passkey/begin", nil)
	defer begin.Body.Close()
	if begin.StatusCode != 200 {
		t.Fatalf("begin answered %d", begin.StatusCode)
	}
	parsed, err := virtualwebauthn.ParseAttestationOptions(readBody(t, begin))
	if err != nil {
		t.Fatalf("parse the creation options: %v", err)
	}
	attestation := virtualwebauthn.CreateAttestationResponse(rp, auth, cred, *parsed)

	finish := s.postAuthedJSON(t, "/password/passkey/finish", map[string]string{"name": "   "}, attestation)
	defer finish.Body.Close()

	if n := s.factorCount(); n != 0 {
		t.Errorf("factorCount() = %d, want 0 — a passkey with no real name was still stored", n)
	}
}

// TestEnrollingAPasskeyMintsRecoveryCodesWhenItIsTheFirstFactor.
//
// Recovery codes were minted only on the TOTP path, because that was the only
// path. An account whose single factor is a passkey on a lost phone must not be
// an account with no way back.
func TestEnrollingAPasskeyMintsRecoveryCodesWhenItIsTheFirstFactor(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"))
	if len(s.cfg.RecoveryCodes()) != 0 {
		t.Fatal("the fixture already has recovery codes; the test cannot tell what minted them")
	}

	enrolPasskey(t, s, "first")

	if len(s.cfg.RecoveryCodes()) == 0 {
		t.Fatal("no recovery codes were minted for an account whose only factor is a passkey")
	}

	before := append([]string(nil), s.cfg.RecoveryCodes()...)
	enrolPasskey(t, s, "second")
	if !slices.Equal(before, s.cfg.RecoveryCodes()) {
		t.Error("enrolling a second passkey re-minted the codes; the operator's printout is now wrong")
	}
}

// TestEnrollingReStampsTheActingSession.
//
// credentialFingerprint covers the passkey set from this release on, so
// enrolling one invalidates every session — including the one enrolling. Without
// a re-stamp the operator is thrown out in the middle of the step the gate just
// forced them into, which reads as a bug in the login rather than a feature.
func TestEnrollingReStampsTheActingSession(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"))
	cookie := s.signIn(t)

	enrolPasskeyWithCookie(t, s, "first", cookie)

	resp := s.getWithCookie(t, "/dashboard", cookie)
	defer resp.Body.Close()
	if resp.StatusCode == 303 {
		t.Fatal("the session that enrolled the passkey was invalidated by its own enrolment")
	}
}

// TestRemovingAPasskeyEndsSessions is the other half: the fingerprint must
// actually cover the set, or removing a lost device leaves its sessions open.
func TestRemovingAPasskeyEndsSessions(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"),
		withTOTP("JBSWY3DPEHPK3PXP")) // a second factor, so removal is allowed
	id := enrolPasskey(t, s, "the lost one")

	other := s.signIn(t)
	removeResp := s.postAuthed(t, "/password/passkey/remove", map[string]string{
		"id":               base64.RawURLEncoding.EncodeToString(id),
		"current_password": testPassword,
	})
	removeResp.Body.Close()

	resp := s.getWithCookie(t, "/dashboard", other)
	defer resp.Body.Close()
	if resp.StatusCode != 303 {
		t.Error("a session open before the passkey was removed still works")
	}
}

// TestTheLastPasskeyCannotBeRemoved is the passkey side of "the last factor
// cannot be removed" — mayRemoveFactor's own invariant, and the one no
// existing test actually watched fail here: TestRemovingAPasskeyEndsSessions
// enrols TOTP specifically so mayRemoveFactor allows the removal, which means
// it can only ever observe the gate agreeing to act — never refusing. With no
// other factor enrolled, removing the account's only passkey must be refused.
func TestTheLastPasskeyCannotBeRemoved(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"))
	id := enrolPasskey(t, s, "the only one")

	resp := s.postAuthed(t, "/password/passkey/remove", map[string]string{
		"id":               base64.RawURLEncoding.EncodeToString(id),
		"current_password": testPassword,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("remove answered %d, want a redirect", resp.StatusCode)
	}

	if got := s.passkeys.all(); len(got) != 1 {
		t.Errorf("%d passkeys remain, want 1 — the account's only factor was removed", len(got))
	}
	if n := s.factorCount(); n != 1 {
		t.Errorf("factorCount() = %d after a refused removal, want 1", n)
	}
}

// TestRemovingAnUnknownPasskeyDoesNothing.
//
// passkeyStore.remove treats an absent id as the desired end state already
// reached — reasonable there, since a caller asking to delete something
// already gone gets what it wanted. The handler has to know the difference:
// a stale page, a resubmitted form, or a bogus id must not claim success —
// removing an id that changes nothing about the enrolled set leaves the
// fingerprint unchanged too, so a wrongly "successful" removal would not
// even show up as an ended session; the flash is the only place this is
// observable at all.
func TestRemovingAnUnknownPasskeyDoesNothing(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"),
		withTOTP("JBSWY3DPEHPK3PXP")) // a second factor, so mayRemoveFactor is not what refuses this
	enrolPasskey(t, s, "the real one")
	before := s.passkeys.all()

	resp := s.postAuthed(t, "/password/passkey/remove", map[string]string{
		"id":               base64.RawURLEncoding.EncodeToString([]byte("not-a-real-credential-id")),
		"current_password": testPassword,
	})
	defer resp.Body.Close()

	if got := s.passkeys.all(); len(got) != len(before) || got[0].Name != before[0].Name {
		t.Errorf("the store changed after removing an id it never held: got %v, want %v", got, before)
	}

	var flashCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == SessionName {
			flashCookie = c
		}
	}
	if flashCookie == nil {
		t.Fatal("no session cookie set on the response")
	}
	sessReq := httptest.NewRequest("GET", "/", nil)
	sessReq.AddCookie(flashCookie)
	sess, err := s.store.Get(sessReq, SessionName)
	if err != nil {
		t.Fatal(err)
	}
	if flash, _ := sess.Values["flash"].(string); flash != "passkey_not_found" {
		t.Errorf("flash = %q, want passkey_not_found — nothing was removed", flash)
	}
}
