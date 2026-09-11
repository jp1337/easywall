package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/descope/virtualwebauthn"

	"github.com/jp1337/easywall/internal/shared"
)

// currentPassword is the form body every /password/passkey/begin call now
// carries: the route is gated on the current password, the same way
// /password/2fa/begin is. Sent even by the tests that expect a refusal for
// some other reason — a begin refused because no password was supplied would
// satisfy their assertions while proving nothing about the check each one is
// actually named after.
var currentPassword = map[string]string{"current_password": testPassword}

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

// sessionCookie returns the value of the session cookie a response set, or ""
// if it set none — for asserting a route did *not* grant a session, which is
// half of what the login passkey tests below have to prove.
//
// Only safe where the route under test never calls setFlash on the path being
// checked: setFlash rides the very same SessionName cookie to carry a
// one-time message to an otherwise-anonymous visitor, so its presence alone
// is not proof of a grant wherever a flash could have set it too. Use
// sessionGrantsAccess instead on any path that might.
// The last match wins, not the first: a browser's cookie jar applies Set-Cookie
// headers in order, so a response that saves the session twice leaves the jar
// holding the second value. handleLoginVerifyPOST's recovery branch does exactly
// that — setFlashN("recovery_left") saves, then grantSession saves again — and
// reading the first header there reports the flash-only cookie, which carries no
// user. That made a correct login look like a refusal.
func sessionCookie(resp *http.Response) string {
	val := ""
	for _, c := range resp.Cookies() {
		if c.Name == SessionName {
			val = c.Value
		}
	}
	return val
}

// sessionGrantsAccess reports whether resp's session cookie actually
// authenticates — sessionUser() returns a username — rather than merely being
// present. Found necessary the hard way: a refused passkey assertion still
// sets a SessionName cookie, because setFlash's "you were told verify_failed"
// rides the same cookie as a real login does; a bare Cookies() scan cannot
// tell the two apart.
func sessionGrantsAccess(t *testing.T, s *Server, resp *http.Response) bool {
	t.Helper()
	val := sessionCookie(resp)
	if val == "" {
		return false
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionName, Value: val})
	sess, err := s.store.Get(req, SessionName)
	if err != nil {
		return false
	}
	return sessionUser(sess, s.currentCredential()) != ""
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
	return enrolPasskeyCredential(t, s, name, cookie).ID
}

// enrolPasskeyCredential is enrolPasskeyWithCookie's own implementation,
// factored out so a caller that also needs to drive a *login* assertion
// against the same device afterwards — TestAPasskeyCompletesTheSecondStep,
// chiefly — can get at the virtualwebauthn.Credential itself. The private key
// lives there, not in anything the server ever hands back, so the ID alone
// (what enrolPasskeyWithCookie's callers have needed until now) is not enough
// to sign a second ceremony against the same enrolled device.
func enrolPasskeyCredential(t *testing.T, s *Server, name string, cookie *http.Cookie) virtualwebauthn.Credential {
	t.Helper()

	begin := doFormRequest(s, "POST", "/password/passkey/begin",
		url.Values{"current_password": {testPassword}}.Encode(), cookie).Result()
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
	return cred
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

	resp := s.postAuthed(t, "/password/passkey/begin", currentPassword)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Errorf("begin answered %d, want %d — with a correct password supplied, the "+
			"missing Relying Party ID must be what refuses it", resp.StatusCode, http.StatusPreconditionFailed)
	}
}

// TestPasskeysAreRefusedInDemoMode — the demo runs behind a real domain, so
// passkeys would otherwise work, and any visitor could leave one behind.
func TestPasskeysAreRefusedInDemoMode(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"), withDemo())

	if got := s.passkeyUnavailableReason(); got != "passkey_demo" {
		t.Errorf("reason = %q, want passkey_demo", got)
	}
	resp := s.postAuthed(t, "/password/passkey/begin", currentPassword)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Errorf("begin answered %d, want %d — with a correct password supplied, the demo "+
			"must be what refuses it", resp.StatusCode, http.StatusPreconditionFailed)
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

	resp := s.postAuthed(t, "/password/passkey/begin", currentPassword)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Errorf("begin answered %d, want %d — with a correct password supplied, the "+
			"self-signed certificate must be what refuses it", resp.StatusCode, http.StatusPreconditionFailed)
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

	begin := s.postAuthed(t, "/password/passkey/begin", currentPassword)
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

	begin := s.postAuthed(t, "/password/passkey/begin", currentPassword)
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

	begin := s.postAuthed(t, "/password/passkey/begin", currentPassword)
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

// TestPasskeyFinishRefusesAnOverLongName covers the other half of
// handlePasskeyFinish's `name == "" || utf8.RuneCountInString(name) >
// maxPasskeyNameLen` — untested before this: TestPasskeyFinishRefusesAnEmptyName
// only ever exercises the left side, so a change to the length half, or to
// the constant itself, could pass every existing test while the bound quietly
// stopped applying.
//
// The fixture is 65 runes of a two-byte character, not 65 ASCII bytes: for
// ASCII, a rune count and a byte count are the same number, so an ASCII
// fixture cannot tell a rune-counting bound from a byte-counting one apart —
// either would reject it identically. Multi-byte runes make the 65 explicit
// as a *character* count, matching utf8.RuneCountInString rather than len().
func TestPasskeyFinishRefusesAnOverLongName(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"))
	auth := virtualwebauthn.NewAuthenticator()
	rp := virtualwebauthn.RelyingParty{
		Name:   "easywall",
		ID:     "firewall.example.org",
		Origin: "https://firewall.example.org",
	}
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	begin := s.postAuthed(t, "/password/passkey/begin", currentPassword)
	defer begin.Body.Close()
	if begin.StatusCode != 200 {
		t.Fatalf("begin answered %d", begin.StatusCode)
	}
	parsed, err := virtualwebauthn.ParseAttestationOptions(readBody(t, begin))
	if err != nil {
		t.Fatalf("parse the creation options: %v", err)
	}
	attestation := virtualwebauthn.CreateAttestationResponse(rp, auth, cred, *parsed)

	name := strings.Repeat("é", maxPasskeyNameLen+1)
	finish := s.postAuthedJSON(t, "/password/passkey/finish", map[string]string{"name": name}, attestation)
	defer finish.Body.Close()

	if n := s.factorCount(); n != 0 {
		t.Errorf("factorCount() = %d, want 0 — a passkey with a %d-rune name was still stored", n, utf8.RuneCountInString(name))
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

// ---------------------------------------------------------------------------
// Signing in with a passkey — the second step, never the first.
// ---------------------------------------------------------------------------

// TestSigningInWithAPasskeyNeedsThePasswordFirst is the whole shape of the
// feature: a passkey is a second factor, never a replacement.
//
// Without the password step there is no pendingLogin, and a passkey assertion
// on its own must not produce a session — otherwise a stolen authenticator is
// a login to a firewall. Both routes are checked: begin, because that is the
// one a real browser reaches with a click, and finish, because a request
// built by hand or by a script can reach it directly and skip begin entirely.
func TestSigningInWithAPasskeyNeedsThePasswordFirst(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"))
	enrolPasskey(t, s, "the one")

	// No password step: no pending cookie of any kind.
	begin := doFormRequest(s, "POST", "/login/passkey/begin", "").Result()
	if begin.StatusCode == 200 {
		t.Fatal("a passkey ceremony began with no password step behind it")
	}
	if sessionCookie(begin) != "" {
		t.Fatal("begin issued a session with no password step behind it")
	}

	finish := doFormRequest(s, "POST", "/login/passkey/finish", "credential=not-a-real-assertion").Result()
	if sessionCookie(finish) != "" {
		t.Fatal("finish issued a session with no password step behind it")
	}
	if finish.StatusCode != http.StatusSeeOther {
		t.Fatalf("finish answered %d, want a redirect (to /login, with nothing consumed)", finish.StatusCode)
	}
}

// TestAPasskeyCompletesTheSecondStep drives the real assertion: password,
// then a virtual authenticator's response to /login/passkey/begin, verified by
// /login/passkey/finish, and checks every part of what a completed login
// promises — a session, the signature counter persisted, and nothing else
// changed about the pending state that a wrong guess would have touched.
func TestAPasskeyCompletesTheSecondStep(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"))
	cred := enrolPasskeyCredential(t, s, "the one", makeAuthCookie(t, s))

	// The password step. hasSecondFactor() must see the passkey even with no
	// TOTP secret set — the fix handleLoginPOST needed — or this never leaves
	// a pendingLogin behind for the passkey route to complete.
	loginResp := doFormRequest(s, "POST", "/login",
		url.Values{"username": {"admin"}, "password": {testPassword}}.Encode()).Result()
	if loginResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login answered %d, want %d", loginResp.StatusCode, http.StatusSeeOther)
	}
	if sessionCookie(loginResp) != "" {
		t.Fatal("the password step alone granted a session on a passkey-only account")
	}
	var pending *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == pendingCookieName {
			pending = c
		}
	}
	if pending == nil {
		t.Fatal("the password step set no pending cookie")
	}

	begin := doFormRequest(s, "POST", "/login/passkey/begin", "", pending).Result()
	if begin.StatusCode != 200 {
		t.Fatalf("begin answered %d: %s", begin.StatusCode, readBody(t, begin))
	}
	var challenge *http.Cookie
	for _, c := range begin.Cookies() {
		if c.Name == loginPasskeyPendingCookieName {
			challenge = c
		}
	}
	if challenge == nil {
		t.Fatal("begin did not set the login passkey challenge cookie")
	}

	parsed, err := virtualwebauthn.ParseAssertionOptions(readBody(t, begin))
	if err != nil {
		t.Fatalf("parse the assertion options: %v", err)
	}

	// The authenticator's counter moved since enrolment — set by hand, since
	// the virtual authenticator does not increment it on its own. This is what
	// TestAPasskeyLoginPersistsTheSignatureCounter below checks was stored.
	cred.Counter = 7
	auth := virtualwebauthn.NewAuthenticator()
	rp := virtualwebauthn.RelyingParty{
		Name:   "easywall",
		ID:     s.cfg.Hostname(),
		Origin: s.publicOrigin(),
	}
	assertion := virtualwebauthn.CreateAssertionResponse(rp, auth, cred, *parsed)

	finish := doFormRequest(s, "POST", "/login/passkey/finish",
		url.Values{"credential": {assertion}}.Encode(), pending, challenge).Result()
	if finish.StatusCode != http.StatusSeeOther {
		t.Fatalf("finish answered %d: %s", finish.StatusCode, readBody(t, finish))
	}
	if loc := finish.Header.Get("Location"); loc != "/dashboard" {
		t.Errorf("finish redirected to %q, want /dashboard", loc)
	}
	if sessionCookie(finish) == "" {
		t.Fatal("a verified passkey assertion did not grant a session")
	}

	stored := s.passkeys.all()
	if len(stored) != 1 {
		t.Fatalf("%d passkeys stored, want 1", len(stored))
	}
	if got := stored[0].Credential.Authenticator.SignCount; got != 7 {
		t.Errorf("stored signature counter = %d, want 7 — the login did not persist it", got)
	}
}

// TestTheSixteenthPasskeyAttemptDoesNotGetThrough mirrors
// TestLoginVerify_TheSixteenthCodeAttemptDoesNotGetThrough exactly, against the
// other door: this is the outer arithmetic — pendingMaxAttempts per
// intermediate state, a new one costing a password round, all of it capped by
// LoginRateLimit at 5 password rounds per 10 minutes per address. No real
// ceremony is driven here on purpose — the finish route fails before it would
// even reach one (no challenge cookie is ever sent), which is the failure this
// test wants: the point under test is the outer bound, not WebAuthn
// verification itself, which TestAPasskeyCompletesTheSecondStep covers.
//
// This test alone does not prove p.Attempts is actually incremented — its own
// twin on the code side does not either, verified by mutation below: with
// `p.Attempts++` deleted, both this test and
// TestLoginVerify_TheSixteenthCodeAttemptDoesNotGetThrough still pass, because
// neither one reuses a single pendingLogin cookie past pendingMaxAttempts —
// each outer round fetches a fresh one regardless of what the previous round's
// server-side state was. TestThreePasskeyFailuresEndTheAttempt below is the
// one that actually watches p.Attempts move, the passkey twin of
// TestLoginVerify_ThreeWrongCodesEndTheAttempt, and it is the one the mutation
// turns red.
func TestTheSixteenthPasskeyAttemptDoesNotGetThrough(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword(testPassword)
	s.cfg.Password = hash
	enrol(t, s)

	const addr = "203.0.113.201:44444"
	post := func(path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = addr // one address for the whole run, unlike doFormRequest
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		s.router.ServeHTTP(rec, req)
		return rec
	}

	attempts := 0
	for round := 1; round <= 6; round++ {
		login := post("/login", "username=admin&password="+url.QueryEscape(testPassword))
		if login.Code == http.StatusTooManyRequests {
			if attempts != pendingMaxAttempts*5 {
				t.Fatalf("the limiter refused password round %d after %d passkey attempts, want %d",
					round, attempts, pendingMaxAttempts*5)
			}
			return
		}
		cookies := login.Result().Cookies()
		for i := 0; i < pendingMaxAttempts; i++ {
			rec := post("/login/passkey/finish", "credential=not-a-real-assertion", cookies...)
			attempts++
			if attempts > pendingMaxAttempts*5 {
				t.Fatalf("passkey attempt %d got through; the second step's budget is not shared", attempts)
			}
			if c := rec.Result().Cookies(); len(c) > 0 {
				cookies = c
			}
		}
	}
	t.Fatalf("six password rounds were allowed; the limiter is not bounding the passkey door "+
		"(%d passkey attempts)", attempts)
}

// TestThreePasskeyFailuresEndTheAttempt is the passkey twin of
// TestLoginVerify_ThreeWrongCodesEndTheAttempt: pendingMaxAttempts failed
// assertions against the *same* pendingLogin cookie, and the last of them must
// redirect to /login (the state cleared), not back to /login/verify (still
// good for another try). This is the test that actually observes p.Attempts
// move — TestTheSixteenthPasskeyAttemptDoesNotGetThrough's own comment
// explains why counting outer rounds cannot.
//
// The cookie the password step issued is presented unchanged every round. It
// used to be replaced with whatever the response carried, which modelled a
// cooperating browser and could not fail for an attacker; see
// TestLoginVerify_ThreeWrongCodesEndTheAttempt and
// TestPending_AFrozenCookieDoesNotBuyMoreAttempts.
func TestThreePasskeyFailuresEndTheAttempt(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	hash, _ := HashPassword(testPassword)
	s.cfg.Password = hash
	enrol(t, s)

	first := doFormRequest(s, "POST", "/login",
		"username=admin&password="+url.QueryEscape(testPassword))
	cookies := first.Result().Cookies()

	for i := 1; i <= pendingMaxAttempts; i++ {
		rec := doFormRequest(s, "POST", "/login/passkey/finish", "credential=not-a-real-assertion", cookies...)
		want := "/login/verify"
		if i == pendingMaxAttempts {
			want = "/login"
		}
		if loc := rec.Header().Get("Location"); loc != want {
			t.Fatalf("passkey failure %d redirected to %q, want %q", i, loc, want)
		}
	}
}

// TestRecoveryCodeStillWorksWithAPasskeyEnrolled is the way back this release
// has already fixed twice for the same reason: an operator whose authenticator
// is lost still has to be able to sign in. The recovery codes here are minted
// and saved directly, not through the passkey ceremony's own auto-mint, so the
// plaintext is in hand to submit — see handlePasskeyFinish's own comment for
// why enrolling with codes already present skips minting a second set.
func TestRecoveryCodeStillWorksWithAPasskeyEnrolled(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"))
	plain, hashes, err := newRecoveryCodes()
	if err != nil {
		t.Fatalf("newRecoveryCodes: %v", err)
	}
	if err := s.cfg.SaveRecoveryCodes(hashes); err != nil {
		t.Fatalf("SaveRecoveryCodes: %v", err)
	}
	enrolPasskey(t, s, "the one")

	loginResp := doFormRequest(s, "POST", "/login",
		url.Values{"username": {"admin"}, "password": {testPassword}}.Encode()).Result()
	if loginResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login answered %d", loginResp.StatusCode)
	}
	var pending *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == pendingCookieName {
			pending = c
		}
	}
	if pending == nil {
		t.Fatal("login set no pending cookie")
	}

	verify := doFormRequest(s, "POST", "/login/verify",
		url.Values{"code": {plain[0]}}.Encode(), pending).Result()
	if verify.StatusCode != http.StatusSeeOther {
		t.Fatalf("verify answered %d: %s", verify.StatusCode, readBody(t, verify))
	}
	if !sessionGrantsAccess(t, s, verify) {
		t.Fatal("a recovery code did not grant a session on an account with a passkey also enrolled")
	}
}

// TestAPasskeyReplayedCounterIsRefused drives two real assertions from the
// same virtual authenticator against the same enrolled credential, without
// advancing the counter between them — exactly what a cloned authenticator, or
// a captured response replayed outright, would produce. go-webauthn's own
// ValidateLogin returns success either way; the counter not moving is the only
// signal there is, in cred.Authenticator.CloneWarning, and this is the test
// that watches easywall actually act on it: the second assertion must be
// refused, must cost the shared attempt budget, and must write
// passkey_clone_suspected rather than passkey_used.
func TestAPasskeyReplayedCounterIsRefused(t *testing.T) {
	fc := newFakeCore(t)
	seen := make(chan shared.Command, 8)
	fc.OnCommand(shared.CmdLogEvent, func(c shared.Command) { seen <- c })

	s := newTestServer(t, fc)
	s.cfg.TLS.Hostname = "firewall.example.org"
	s.cfg.TLS.CertFile = "test-fixture-cert.pem"

	cred := enrolPasskeyCredential(t, s, "the one", makeAuthCookie(t, s))

	// Drain the enrolment's own event before the two logins below — otherwise
	// it is the first thing the channel yields and is mistaken for the first
	// login's.
	select {
	case cmd := <-seen:
		var p shared.LogEventPayload
		_ = json.Unmarshal(cmd.Payload, &p)
		if p.Event != shared.EvPasskeyEnrolled {
			t.Fatalf("enrolment recorded %q, want %q", p.Event, shared.EvPasskeyEnrolled)
		}
	case <-time.After(time.Second):
		t.Fatal("no event recorded for the enrolment")
	}

	rp := virtualwebauthn.RelyingParty{
		Name:   "easywall",
		ID:     s.cfg.Hostname(),
		Origin: s.publicOrigin(),
	}
	auth := virtualwebauthn.NewAuthenticator()

	doLogin := func() *http.Response {
		t.Helper()
		loginResp := doFormRequest(s, "POST", "/login",
			url.Values{"username": {"admin"}, "password": {testPassword}}.Encode()).Result()
		if loginResp.StatusCode != http.StatusSeeOther {
			t.Fatalf("login answered %d", loginResp.StatusCode)
		}
		var pending *http.Cookie
		for _, c := range loginResp.Cookies() {
			if c.Name == pendingCookieName {
				pending = c
			}
		}
		if pending == nil {
			t.Fatal("login set no pending cookie")
		}

		begin := doFormRequest(s, "POST", "/login/passkey/begin", "", pending).Result()
		if begin.StatusCode != 200 {
			t.Fatalf("begin answered %d: %s", begin.StatusCode, readBody(t, begin))
		}
		var challenge *http.Cookie
		for _, c := range begin.Cookies() {
			if c.Name == loginPasskeyPendingCookieName {
				challenge = c
			}
		}
		if challenge == nil {
			t.Fatal("begin did not set the login passkey challenge cookie")
		}
		parsed, err := virtualwebauthn.ParseAssertionOptions(readBody(t, begin))
		if err != nil {
			t.Fatalf("parse the assertion options: %v", err)
		}

		assertion := virtualwebauthn.CreateAssertionResponse(rp, auth, cred, *parsed)
		return doFormRequest(s, "POST", "/login/passkey/finish",
			url.Values{"credential": {assertion}}.Encode(), pending, challenge).Result()
	}

	// First assertion: the counter advances from 0 (set at enrolment) to 5.
	// Legitimate, and it must succeed and persist 5.
	cred.Counter = 5
	first := doLogin()
	defer first.Body.Close()
	if first.StatusCode != http.StatusSeeOther || !sessionGrantsAccess(t, s, first) {
		t.Fatalf("the first, legitimate assertion did not grant a session (status %d)", first.StatusCode)
	}
	if got := s.passkeys.all()[0].Credential.Authenticator.SignCount; got != 5 {
		t.Fatalf("stored counter after the first login = %d, want 5", got)
	}

	// Second assertion: the SAME counter value again, not advanced — a cloned
	// authenticator that last saw 5 too, or the first response replayed
	// outright. Must be refused, not granted.
	second := doLogin()
	defer second.Body.Close()
	if sessionGrantsAccess(t, s, second) {
		t.Fatal("a replayed/cloned counter granted a session")
	}
	if loc := second.Header.Get("Location"); loc != "/login/verify" {
		t.Errorf("refused assertion redirected to %q, want /login/verify (one of pendingMaxAttempts, not the last)", loc)
	}
	if got := s.passkeys.all()[0].Credential.Authenticator.SignCount; got != 5 {
		t.Errorf("stored counter after the refused replay = %d, want unchanged at 5", got)
	}

	// Drain the first login's own event (passkey_used) before checking the
	// second's.
	select {
	case cmd := <-seen:
		var p shared.LogEventPayload
		_ = json.Unmarshal(cmd.Payload, &p)
		if p.Event != shared.EvPasskeyUsed {
			t.Fatalf("first login recorded %q, want %q", p.Event, shared.EvPasskeyUsed)
		}
	case <-time.After(time.Second):
		t.Fatal("no event recorded for the first, legitimate login")
	}
	select {
	case cmd := <-seen:
		var p shared.LogEventPayload
		_ = json.Unmarshal(cmd.Payload, &p)
		if p.Event != shared.EvPasskeyCloneSuspected {
			t.Errorf("second login recorded %q, want %q", p.Event, shared.EvPasskeyCloneSuspected)
		}
	case <-time.After(time.Second):
		t.Fatal("no event recorded for the replayed/cloned counter")
	}
}

// TestPasskeyEnrolmentNeedsTheCurrentPassword — /password/passkey/begin was the
// one credential write on this page a session alone could perform. Every other
// control here (POST /password, /password/2fa/begin, /2fa/disable,
// /2fa/recovery, /passkey/remove) re-asks for the password, because a session
// is not proof that the operator is the one holding it: a stolen cookie or an
// unlocked browser is enough. Enrolling a passkey through it plants a durable
// second factor, and handlePasskeyFinish's restampSession then stamps the
// thief's session with the new fingerprint and ends the real operator's.
//
// Asserted three ways, because a redirect alone would be satisfied by a route
// that refused for any reason at all: the flash names the password, no
// challenge cookie is issued (which is what makes Finish unreachable), and
// nothing reaches the store.
func TestPasskeyEnrolmentNeedsTheCurrentPassword(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"))

	for _, tc := range []struct {
		name string
		form map[string]string
	}{
		{"no password at all", nil},
		{"the wrong password", map[string]string{"current_password": "not-the-password"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := s.postAuthed(t, "/password/passkey/begin", tc.form)
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusSeeOther {
				t.Fatalf("begin answered %d, want %d — a session alone started a registration ceremony",
					resp.StatusCode, http.StatusSeeOther)
			}
			if loc := resp.Header.Get("Location"); loc != "/password" {
				t.Errorf("begin redirected to %q, want /password", loc)
			}
			for _, c := range resp.Cookies() {
				if c.Name == passkeyPendingCookieName && c.MaxAge >= 0 && c.Value != "" {
					t.Error("a refused begin still issued a challenge cookie — finish would inherit no gate at all")
				}
			}
			if got := flashFrom(t, s, resp); got != "password_wrong" {
				t.Errorf("flash = %q, want password_wrong", got)
			}
			if n := len(s.passkeys.all()); n != 0 {
				t.Errorf("%d passkeys stored, want 0", n)
			}
		})
	}

	// The same route, the right password: the gate refuses the session that
	// cannot prove itself and nothing else. Without this the test above would
	// still pass against a handler that refused every caller.
	ok := s.postAuthed(t, "/password/passkey/begin", currentPassword)
	defer ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("begin with the correct password answered %d, want 200", ok.StatusCode)
	}
}

// flashFrom reads the one-time message a response set, by replaying its session
// cookie into the store the same way the next render would.
func flashFrom(t *testing.T, s *Server, resp *http.Response) string {
	t.Helper()
	val := sessionCookie(resp)
	if val == "" {
		return ""
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionName, Value: val})
	sess, err := s.store.Get(req, SessionName)
	if err != nil {
		return ""
	}
	msg, _ := sess.Values["flash"].(string)
	return msg
}

// TestAReplayedPasskeyAssertionIsRefused — the same assertion bytes, with the
// same retained easywall_pending and easywall_login_passkey cookies, submitted
// twice.
//
// Both cookies are cleared with MaxAge: -1, which instructs a browser and
// nobody else; pendingLogin is entirely cookie-borne, so a party holding its
// own copies simply sends them again. The clone check is not a backstop here
// and this test is built so that it cannot be mistaken for one: the credential's
// counter stays at 0, which is exactly what iCloud Keychain and most platform
// authenticators report on every assertion, and go-webauthn's UpdateCounter
// deliberately exempts authDataCount == 0 && SignCount == 0 from the
// CloneWarning it would otherwise raise. So the second submission verifies
// cleanly and the only thing that can refuse it is the server remembering that
// it already spent that challenge.
//
// TestAPasskeyReplayedCounterIsRefused above is the other half of the pair: a
// *fresh* challenge signed by a device whose counter did not move. This one is
// the identical response, sent twice.
func TestAReplayedPasskeyAssertionIsRefused(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"))
	cred := enrolPasskeyCredential(t, s, "the one", makeAuthCookie(t, s))
	cred.Counter = 0 // a platform passkey: zero on every assertion, forever

	login := doFormRequest(s, "POST", "/login",
		url.Values{"username": {"admin"}, "password": {testPassword}}.Encode()).Result()
	defer login.Body.Close()
	var pending *http.Cookie
	for _, c := range login.Cookies() {
		if c.Name == pendingCookieName {
			pending = c
		}
	}
	if pending == nil {
		t.Fatal("the password step set no pending cookie")
	}

	begin := doFormRequest(s, "POST", "/login/passkey/begin", "", pending).Result()
	if begin.StatusCode != http.StatusOK {
		t.Fatalf("begin answered %d", begin.StatusCode)
	}
	var challenge *http.Cookie
	for _, c := range begin.Cookies() {
		if c.Name == loginPasskeyPendingCookieName {
			challenge = c
		}
	}
	if challenge == nil {
		t.Fatal("begin did not set the login passkey challenge cookie")
	}
	parsed, err := virtualwebauthn.ParseAssertionOptions(readBody(t, begin))
	if err != nil {
		t.Fatalf("parse the assertion options: %v", err)
	}
	assertion := virtualwebauthn.CreateAssertionResponse(rpFor(s), virtualwebauthn.NewAuthenticator(), cred, *parsed)

	// Identical bytes, identical cookies, twice — what a captured exchange
	// gives a replayer, with nothing recomputed between the two.
	send := func() *http.Response {
		t.Helper()
		return doFormRequest(s, "POST", "/login/passkey/finish",
			url.Values{"credential": {assertion}}.Encode(), pending, challenge).Result()
	}

	first := send()
	defer first.Body.Close()
	if !sessionGrantsAccess(t, s, first) {
		t.Fatalf("the first, legitimate assertion did not grant a session (status %d, %s)",
			first.StatusCode, first.Header.Get("Location"))
	}

	second := send()
	defer second.Body.Close()
	if sessionGrantsAccess(t, s, second) {
		t.Fatal("the identical assertion was replayed and granted a session a second time")
	}
	if loc := second.Header.Get("Location"); loc != "/login/verify" {
		t.Errorf("the replay redirected to %q, want /login/verify — it must cost an attempt, "+
			"not merely fail quietly", loc)
	}
}

// TestAReplayedPasskeyEnrolmentIsRefused — the enrolment half of the same
// hole. handlePasskeyFinish's challenge cookie is cleared the same
// browser-only way, so the same retained pair enrols the same credential
// twice: two entries, one device, and factorCount() then reads 2 where
// mayRemoveFactor has one real device to protect. Refused on the second
// submission because the challenge is already spent.
func TestAReplayedPasskeyEnrolmentIsRefused(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"))
	cookie := makeAuthCookie(t, s)

	begin := doFormRequest(s, "POST", "/password/passkey/begin",
		url.Values{"current_password": {testPassword}}.Encode(), cookie).Result()
	if begin.StatusCode != http.StatusOK {
		t.Fatalf("begin answered %d", begin.StatusCode)
	}
	var challenge *http.Cookie
	for _, c := range begin.Cookies() {
		if c.Name == passkeyPendingCookieName {
			challenge = c
		}
	}
	if challenge == nil {
		t.Fatal("begin did not set the passkey challenge cookie")
	}
	parsed, err := virtualwebauthn.ParseAttestationOptions(readBody(t, begin))
	if err != nil {
		t.Fatalf("parse the creation options: %v", err)
	}
	attestation := virtualwebauthn.CreateAttestationResponse(
		rpFor(s), virtualwebauthn.NewAuthenticator(), virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2), *parsed)

	vals := url.Values{"name": {"the one"}, "credential": {attestation}}.Encode()
	first := doFormRequest(s, "POST", "/password/passkey/finish", vals, cookie, challenge).Result()
	defer first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("the first enrolment answered %d", first.StatusCode)
	}
	if n := len(s.passkeys.all()); n != 1 {
		t.Fatalf("%d passkeys stored after the first enrolment, want 1", n)
	}

	// handlePasskeyFinish re-stamps the session, so the replayer holds the
	// cookie the first response set — a browser's own jar would. Without this
	// the second request is simply unauthenticated and the test would pass on
	// the wrong refusal entirely.
	for _, c := range first.Cookies() {
		if c.Name == SessionName {
			cookie.Value = c.Value
		}
	}
	second := doFormRequest(s, "POST", "/password/passkey/finish", vals, cookie, challenge).Result()
	defer second.Body.Close()
	if n := len(s.passkeys.all()); n != 1 {
		t.Errorf("%d passkeys stored after replaying the identical enrolment, want 1", n)
	}
	if loc := second.Header.Get("Location"); loc != "/password" {
		t.Errorf("the replayed enrolment answered %d / %q, want a redirect to /password",
			second.StatusCode, loc)
	}
}

// rpFor builds the RelyingParty a virtual authenticator signs against, from
// the fixture's own configuration rather than from repeated literals — the
// three fields have to agree with what webAuthn() built or every ceremony
// fails for a reason that has nothing to do with the test.
func rpFor(s *Server) virtualwebauthn.RelyingParty {
	return virtualwebauthn.RelyingParty{
		Name:   "easywall",
		ID:     s.cfg.Hostname(),
		Origin: s.publicOrigin(),
	}
}

// TestASpentChallengeIsForgottenAfterItsTTL — the map must not grow without
// bound, and an entry past its lifetime must not linger: the cookie carrying
// that challenge stopped being accepted at the same moment.
func TestASpentChallengeIsForgottenAfterItsTTL(t *testing.T) {
	if !spendChallenge("a-challenge-nobody-else-uses", time.Minute) {
		t.Fatal("a fresh challenge was reported as already spent")
	}
	if spendChallenge("a-challenge-nobody-else-uses", time.Minute) {
		t.Fatal("the same challenge was accepted twice inside its own TTL")
	}

	// A second entry, given a TTL that has run out by the time anything looks
	// at it again.
	if !spendChallenge("a-short-lived-challenge", time.Millisecond) {
		t.Fatal("a fresh challenge was reported as already spent")
	}
	time.Sleep(3 * time.Millisecond)
	// The sweep rides on the next insert, so one unrelated call runs it.
	spendChallenge("an-unrelated-challenge", time.Minute)
	spentChallenges.mu.Lock()
	_, still := spentChallenges.at["a-short-lived-challenge"]
	spentChallenges.mu.Unlock()
	if still {
		t.Error("an expired entry was still held — the map grows without bound")
	}
}

// TestAnUnknownChallengeIsUnspent — the fail-open direction, and the one this
// must never get wrong. The map is in memory: a restart empties it, and every
// challenge a running process had spent becomes unknown again. Unknown has to
// mean unspent, never "refuse", or a restart mid-ceremony would be a fifth way
// to be locked out of a firewall.
func TestAnUnknownChallengeIsUnspent(t *testing.T) {
	if !spendChallenge("never-seen-by-this-process", time.Minute) {
		t.Error("a challenge this process has no record of was treated as spent — " +
			"a restart would refuse logins it has no reason to refuse")
	}
}

// TestThePasskeyCardCarriesACurrentPasswordField — the other half of the gate
// handlePasskeyBegin now applies. A server-side check with no field in front of
// it is a dead button, and nothing else in the tree would notice: npm run
// check:ui drives the demo, where passkeyUnavailableReason disables this card
// before the field is ever rendered.
func TestThePasskeyCardCarriesACurrentPasswordField(t *testing.T) {
	s := newPasskeyTestServer(t, withHostname("firewall.example.org"))
	body := doRequest(s, "GET", "/password", nil, makeAuthCookie(t, s)).Body.String()

	if !strings.Contains(body, `id="passkey-name"`) {
		t.Fatal("the passkey enrolment card did not render at all — the fixture is wrong, not the template")
	}
	if !strings.Contains(body, `id="passkey-password" name="current_password"`) {
		t.Error(`the enrolment card has no current_password field; ` +
			`/password/passkey/begin refuses without one, so the button cannot work`)
	}
}
