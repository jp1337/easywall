package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/gorilla/sessions"
)

// Enrolling and removing passkeys.
//
// A passkey is a second factor here and never a replacement for the password:
// the login ceremony Task 13 adds lives at /login/verify, after the password
// step, as an alternative to typing a code. A stolen phone is not a login.
//
// Availability is not a preference. WebAuthn refuses a Relying Party ID that is
// not a registrable domain, and a browser refuses any origin whose
// certificate it does not trust. Both are true of a default easywall
// installation reached over its self-signed IP address, so the interface says
// which one is in the way rather than showing a control that fails with a
// SecurityError only a browser console reveals.

const (
	// passkeyPendingCookieName is its own cookie, distinct from easywall_pending:
	// that one carries a half-finished login (see pending2fa.go), this one a
	// half-finished enrolment, and the two must never be confused about which
	// ceremony a request is in the middle of.
	passkeyPendingCookieName = "easywall_passkey"

	// passkeyPendingLifetime is how long a registration ceremony stays open, in
	// seconds. Longer than the login's own pendingLifetime: unlocking a phone to
	// read six digits is quick, but a platform authenticator's own "insert your
	// key and touch it" prompt can wait on a human for longer, and a browser's
	// own ceremony timeout is already the tighter bound in practice.
	passkeyPendingLifetime = 300

	passkeyPendingDataKey = "d"

	// maxPasskeyNameLen bounds what an operator can call a passkey. Not a
	// storage limit — passkeys.json is JSON on disk and would hold a name of
	// any length without complaint — this is a display and sanity bound: what
	// the card can show on one line, and a check against a name nobody typed
	// on purpose (a script, a pasted blob). Runes, not bytes — same reason
	// passwordPolicyError in auth.go counts runes: a four-byte emoji or an
	// umlaut must not count as several characters against a limit stated in
	// characters. 64 is generous for a device label ("YubiKey on the keyring"
	// is 22) and short enough that the card's layout does not have to plan
	// for arbitrary length.
	maxPasskeyNameLen = 64
)

// newPasskeyPendingStore builds the store for the challenge between
// BeginRegistration and FinishRegistration.
//
// Modelled directly on newPendingStore in auth.go — a cookie, not a
// server-side table, because BeginRegistration runs behind an authenticated
// session but the same reasoning still applies: this is intermediate ceremony
// state with no business outliving the ceremony or appearing in every other
// request's cookie header.
//
//   - Path "/password", so the cookie is sent only to the enrolment routes
//     mounted under it.
//   - store.MaxAge, never Options.MaxAge — see newSessionStore's own comment
//     for why: the codec's own thirty-day default survives a fresh Options
//     struct, so a five-minute cookie the *server* still accepts for a month
//     is the bug newSessionStore was written to close, and would reopen here
//     if repeated with a plain assignment.
func newPasskeyPendingStore(key string) *sessions.CookieStore {
	store := sessions.NewCookieStore([]byte(key))
	store.Options = &sessions.Options{
		Path:     "/password",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	store.MaxAge(passkeyPendingLifetime)
	return store
}

// writePasskeyChallenge stores the ceremony's SessionData for the matching
// Finish call to read back.
//
// webauthn.SessionData is fully JSON-serialisable, so it rides the cookie the
// same way pendingLogin's fields do: marshalled once into a single string
// value, rather than teaching gorilla/sessions' gob encoding about the
// library's types.
func (s *Server) writePasskeyChallenge(w http.ResponseWriter, r *http.Request, sd *webauthn.SessionData) error {
	data, err := json.Marshal(sd)
	if err != nil {
		return fmt.Errorf("encode passkey challenge: %w", err)
	}
	sess, _ := s.passkeyPending.Get(r, passkeyPendingCookieName)
	sess.Values[passkeyPendingDataKey] = string(data)
	return sess.Save(r, w)
}

// readPasskeyChallenge returns the ceremony state the request carries, or
// ok=false when there is none, it is expired, or it does not parse.
func (s *Server) readPasskeyChallenge(r *http.Request) (*webauthn.SessionData, bool) {
	sess, err := s.passkeyPending.Get(r, passkeyPendingCookieName)
	if err != nil || sess.IsNew {
		return nil, false
	}
	raw, ok := sess.Values[passkeyPendingDataKey].(string)
	if !ok || raw == "" {
		return nil, false
	}
	var sd webauthn.SessionData
	if err := json.Unmarshal([]byte(raw), &sd); err != nil {
		return nil, false
	}
	return &sd, true
}

// clearPasskeyChallenge ends the ceremony state. Called once Finish has read
// it, whether the ceremony went on to succeed or not — a challenge is single
// use either way.
func (s *Server) clearPasskeyChallenge(w http.ResponseWriter, r *http.Request) {
	sess, _ := s.passkeyPending.Get(r, passkeyPendingCookieName)
	sess.Options = &sessions.Options{
		Path:     "/password",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	if err := sess.Save(r, w); err != nil {
		slog.Warn("could not clear the passkey ceremony state", "error", err)
	}
}

// passkeyUnavailableReason returns the locale key naming why passkeys cannot
// be used on this installation, or "" when they can.
//
// One reason at a time, most fixable first: an operator told all three at
// once is told none of them usefully. The demo is checked first because it is
// the one a running instance can never fix by editing web.toml — there is
// nothing to point the operator at past "this is the demo". The hostname is
// checked next because it is a precondition of the third check even mattering:
// WebAuthn's Relying Party ID has to be a registrable domain before whether a
// browser trusts the certificate on it is a question worth asking.
//
// The third reason is the one this function can name with real certainty
// despite not knowing what any given browser trusts: an installation with a
// hostname, no ACME, and no operator-supplied certificate is running on the
// self-signed pair easywall generates itself (see CustomCertConfigured), and
// no browser trusts that by default. Leaving this reason unchecked was the
// exact failure this file's own package comment describes: hostname set,
// self-signed cert, the button reads enabled, the ceremony fails in the
// browser with a SecurityError, and the operator sees nothing at all.
func (s *Server) passkeyUnavailableReason() string {
	if s.client.IsDemo() {
		return "passkey_demo"
	}
	if s.cfg.Hostname() == "" {
		return "passkey_no_hostname"
	}
	if !s.cfg.ACMEEnabled() && !s.cfg.CustomCertConfigured() {
		return "passkey_self_signed"
	}
	return ""
}

// webAuthn builds the ceremony driver for this installation. The error is
// non-nil only when no hostname is configured — RPID must be a registrable
// domain, and the library's own Config.validate does not itself reject an
// empty one, so this checks it before asking the library to build anything.
//
// Built per call rather than once at startup, because the hostname is
// configuration and configuration is reloadable — a value captured at
// startup is a value that disagrees with web.toml after an edit.
func (s *Server) webAuthn() (*webauthn.WebAuthn, error) {
	host := s.cfg.Hostname()
	if host == "" {
		return nil, fmt.Errorf("no tls.hostname is configured; WebAuthn requires a registrable domain " +
			"as its Relying Party ID and rejects a bare IP address")
	}
	return webauthn.New(&webauthn.Config{
		RPID:          host,
		RPDisplayName: "easywall",
		// The port is part of the origin and not part of the RP ID. An
		// installation on a non-default port has origin https://host:port and
		// RP ID host — see publicOrigin's own comment.
		RPOrigins: []string{s.publicOrigin()},
	})
}

// passkeyUser adapts easywall's one account to go-webauthn's User interface.
//
// There is exactly one account, so the user handle is derived from the
// username rather than a separately stored random value: it only has to be
// stable across the two halves of one ceremony and across ceremonies, which
// the username already is, and it is never displayed or treated as a secret.
type passkeyUser struct {
	id          []byte
	name        string
	credentials []webauthn.Credential
}

func (u passkeyUser) WebAuthnID() []byte                         { return u.id }
func (u passkeyUser) WebAuthnName() string                       { return u.name }
func (u passkeyUser) WebAuthnDisplayName() string                { return u.name }
func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

// passkeyUser returns the User the current ceremony runs against.
func (s *Server) passkeyUser() passkeyUser {
	username, _ := s.cfg.Credentials()
	all := s.passkeys.all()
	creds := make([]webauthn.Credential, len(all))
	for i, pk := range all {
		creds[i] = pk.Credential
	}
	return passkeyUser{id: []byte(username), name: username, credentials: creds}
}

// handlePasskeyBegin starts a registration ceremony and hands the browser the
// options to feed into navigator.credentials.create().
//
// Gated on availability alone, not on the current password: nothing is
// written until Finish, and Finish's own signature verification — not a
// password re-check — is what a forged request cannot get past.
func (s *Server) handlePasskeyBegin(w http.ResponseWriter, r *http.Request) {
	if reason := s.passkeyUnavailableReason(); reason != "" {
		http.Error(w, reason, http.StatusPreconditionFailed)
		return
	}
	wa, err := s.webAuthn()
	if err != nil {
		http.Error(w, err.Error(), http.StatusPreconditionFailed)
		return
	}

	// Excludes every already-enrolled credential, so the same physical
	// authenticator cannot be enrolled a second time under a different name.
	// Without this, factorCount() and mayRemoveFactor() count entries rather
	// than devices: three enrolments of one key read as three factors, and
	// removing "one of them" leaves the operator believing another still
	// stands between them and the account when it was the same key all along.
	user := s.passkeyUser()
	creation, session, err := wa.BeginRegistration(user,
		webauthn.WithExclusions(webauthn.Credentials(user.credentials).CredentialDescriptors()))
	if err != nil {
		slog.Error("could not begin a passkey registration ceremony", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := s.writePasskeyChallenge(w, r, session); err != nil {
		slog.Error("could not store the passkey registration challenge", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(creation); err != nil {
		slog.Warn("could not write passkey registration options", "error", err)
	}
}

// handlePasskeyFinish verifies the ceremony, stores the credential, and mints
// recovery codes when this was the first factor.
//
// name and credential arrive as ordinary form fields rather than the request
// body: the WebAuthn response protocol.ParseCredentialCreationResponseBody
// reads is itself a complete JSON document, and an HTTP request has only one
// body to be either that document or a form — so credential carries it as a
// string, the same way the browser side builds it from
// navigator.credentials.create()'s result before submitting the enrolment
// form.
func (s *Server) handlePasskeyFinish(w http.ResponseWriter, r *http.Request) {
	if reason := s.passkeyUnavailableReason(); reason != "" {
		s.setFlash(w, r, reason)
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	wa, err := s.webAuthn()
	if err != nil {
		s.setFlash(w, r, "passkey_no_hostname")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	sd, ok := s.readPasskeyChallenge(r)
	if !ok {
		s.setFlash(w, r, "passkey_setup_expired")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	// One-shot either way: a failed ceremony does not leave a challenge behind
	// for a second, unrelated attempt to be checked against.
	s.clearPasskeyChallenge(w, r)

	// Required, not defaulted: the brief's own justification for allowing more
	// than one passkey is that "the one I lost" has to be findable, and a
	// silent "Passkey" for every entry defeats that the first time there are
	// two. The client already enforces this — the field is required, and
	// app.js refuses to submit an empty one via the field's own native
	// validation — but the server does not trust that: a request built by
	// hand or by a script skips it entirely.
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" || utf8.RuneCountInString(name) > maxPasskeyNameLen {
		s.setFlash(w, r, "passkey_name_required")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	parsed, err := protocol.ParseCredentialCreationResponseBody(strings.NewReader(r.FormValue("credential")))
	if err != nil {
		slog.Warn("a passkey registration response did not parse", "error", err)
		s.setFlash(w, r, "passkey_ceremony_failed")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	cred, err := wa.CreateCredential(s.passkeyUser(), *sd, parsed)
	if err != nil {
		slog.Warn("a passkey registration ceremony did not verify", "error", err)
		s.setFlash(w, r, "passkey_ceremony_failed")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	// Read before add: once that call succeeds, factorCount is at least one and
	// the question "was this the first factor" can no longer be asked. Mirrors
	// handle2FAConfirm's own wasFirstFactor read for exactly the same reason.
	wasFirstFactor := s.factorCount() == 0

	if err := s.passkeys.add(name, *cred); err != nil {
		slog.Error("could not store the enrolled passkey", "error", err)
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	// Eight codes at the first factor, whichever it is, and only at the first.
	// They were minted only on the TOTP path because that was the only path
	// there was; an account whose single factor is a passkey on a lost phone
	// must not be an account with no way back. The stored-codes check is the
	// same belt-and-suspenders handle2FAConfirm's identical line carries: it
	// changes nothing for the ordinary case wasFirstFactor already covers,
	// and costs nothing when it doesn't fire.
	var plain []string
	if wasFirstFactor && len(s.cfg.RecoveryCodes()) == 0 {
		p, hashes, err := newRecoveryCodes()
		if err != nil {
			slog.Error("could not generate recovery codes", "error", err)
		} else if err := s.cfg.SaveRecoveryCodes(hashes); err != nil {
			slog.Error("could not store recovery codes", "error", err)
		} else {
			plain = p
		}
	}

	// The fingerprint just changed — the passkey set it covers is not the same
	// set it was a moment ago — so every other session ends at this moment.
	// Re-stamp ours, exactly as handle2FAConfirm and a password change already
	// do, or the operator who just enrolled would be thrown out of the very
	// step the gate forced them into.
	s.restampSession(w, r)
	s.setFlash(w, r, "passkey_added")

	page := s.passwordPage(nil, plain)
	page.JustGated = wasFirstFactor
	s.render(w, r, "password.html", "password", page)
}

// handlePasskeyRemove deletes an enrolled passkey.
//
// Gated the same way handle2FADisable gates switching TOTP off: the current
// password, never a second code — whoever is at this form already holds a
// session that came through the second factor, and a further code would only
// create a new lockout case against no attacker it stops. mayRemoveFactor is
// the one rule for whether any factor may be taken away; this does not
// reimplement it.
func (s *Server) handlePasskeyRemove(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentPassword(r) {
		s.setFlash(w, r, "password_wrong")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	if s.client.IsDemo() {
		s.setFlash(w, r, "demo_readonly")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	if !s.mayRemoveFactor() {
		s.setFlash(w, r, "factor_last")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	id, err := base64.RawURLEncoding.DecodeString(r.FormValue("id"))
	if err != nil {
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	// passkeyStore.remove treats an absent id as the desired end state
	// already reached, which is right for that method — a caller asking to
	// remove something already gone gets what it wanted. This handler has to
	// know the difference: a stale page, a resubmitted form, or a bogus id
	// must not restamp the session and end every other one, or flash success
	// for a removal that did not happen.
	found := false
	for _, pk := range s.passkeys.all() {
		if bytes.Equal(pk.ID, id) {
			found = true
			break
		}
	}
	if !found {
		s.setFlash(w, r, "passkey_not_found")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	if err := s.passkeys.remove(id); err != nil {
		slog.Error("could not remove the passkey", "error", err)
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	// The fingerprint just changed — the removed credential is no longer part
	// of the set it covers — so every session that could have signed in with
	// it ends now, including ones already open. Re-stamp ours so the operator
	// who just removed it is not thrown out along with them.
	s.restampSession(w, r)
	s.setFlash(w, r, "passkey_removed")
	http.Redirect(w, r, "/password", http.StatusSeeOther)
}
