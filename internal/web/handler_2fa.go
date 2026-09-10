package web

import (
	"html/template"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// pendingSecretLifetime is how long an unconfirmed secret is held. Long enough
// to unlock a phone, open an app, scan and type; short enough that a browser tab
// left open all afternoon is not still holding one.
const pendingSecretLifetime = 10 * time.Minute

// pendingSecrets holds secrets that have been generated and not yet confirmed,
// keyed by session id.
//
// In memory and not in a cookie, and the reasoning inverts the one behind
// easywall_pending: enrolment is *authenticated*, there is exactly one account,
// so this table never holds more than a handful of entries and no stranger
// reaches it. In exchange, an unconfirmed secret never enters a cookie —
// gorilla/sessions with only a hash key signs but does not encrypt, so a cookie
// value is readable plaintext. It would be on screen as a QR code anyway, so
// this is not a breach; it is merely unnecessary.
//
// A restart mid-enrolment therefore means "start again" rather than "an
// unconfirmed secret is sitting in a browser store". Built on the pattern of
// revokedSessions in sessionrevoke.go.
var pendingSecrets = struct {
	mu sync.Mutex
	at map[string]pendingSecret
}{at: make(map[string]pendingSecret)}

type pendingSecret struct {
	secret string
	issued time.Time

	// failed is whether a submitted code has ever missed against this entry.
	// It gates the recovery-code escape on the setup card the same way
	// pendingFirstRun.Failed gates the wizard's: offered from the first
	// render, it would be the path of least resistance instead of the escape
	// hatch it is. See pendingSecretMarkFailed and handle2FAEnrolUnverified.
	failed bool
}

func pendingSecretStore(sessionID, secret string) {
	if sessionID == "" {
		return
	}
	now := time.Now()
	pendingSecrets.mu.Lock()
	defer pendingSecrets.mu.Unlock()
	for id, p := range pendingSecrets.at {
		if now.Sub(p.issued) > pendingSecretLifetime {
			delete(pendingSecrets.at, id)
		}
	}
	pendingSecrets.at[sessionID] = pendingSecret{secret: secret, issued: now}
}

// pendingSecretLookup returns the secret, whether a code has already failed
// against it, and whether the entry exists and has not aged past its
// lifetime — one lock acquisition for what a caller wanting both the secret
// and the failed flag used to need two separate accessors, and two separate
// locks, to get.
func pendingSecretLookup(sessionID string) (secret string, failed bool, ok bool) {
	if sessionID == "" {
		return "", false, false
	}
	pendingSecrets.mu.Lock()
	defer pendingSecrets.mu.Unlock()
	p, exists := pendingSecrets.at[sessionID]
	if !exists || time.Since(p.issued) > pendingSecretLifetime {
		return "", false, false
	}
	return p.secret, p.failed, true
}

func pendingSecretClear(sessionID string) {
	pendingSecrets.mu.Lock()
	defer pendingSecrets.mu.Unlock()
	delete(pendingSecrets.at, sessionID)
}

// pendingSecretMarkFailed records that a submitted code missed against
// sessionID, so the next render of the setup card offers the recovery-code
// escape. Mirrors firstRunPendingMarkFailed's shape and its guard against
// reviving an entry already past its lifetime.
func pendingSecretMarkFailed(sessionID string) {
	if sessionID == "" {
		return
	}
	pendingSecrets.mu.Lock()
	defer pendingSecrets.mu.Unlock()

	p, ok := pendingSecrets.at[sessionID]
	if !ok || time.Since(p.issued) > pendingSecretLifetime {
		return
	}
	p.failed = true
	pendingSecrets.at[sessionID] = p
}

// sessionID returns the identifier of the session this request carries.
func (s *Server) sessionID(r *http.Request) string {
	sess, err := s.store.Get(r, SessionName)
	if err != nil {
		return ""
	}
	id, _ := sess.Values[SessionIDKey].(string)
	return id
}

// checkCurrentPassword is the gate on all four routes — the same rule the page
// already applies to changing the password.
func (s *Server) checkCurrentPassword(r *http.Request) bool {
	_, hash := s.cfg.Credentials()
	return VerifyPassword(r.FormValue("current_password"), hash)
}

// handle2FABegin generates a secret and shows it. Nothing is stored.
func (s *Server) handle2FABegin(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentPassword(r) {
		s.setFlash(w, r, "password_wrong")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	secret, err := newTOTPSecret()
	if err != nil {
		slog.Error("could not generate a TOTP secret", "error", err)
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	pendingSecretStore(s.sessionID(r), secret)

	// Rendered as the response to this POST rather than redirected to a GET, so
	// a reload cannot mint a second secret and the page cannot be bookmarked.
	s.renderSetupAgain(w, r, secret)
}

// handle2FAConfirm stores the secret and the eight hashes in one write.
func (s *Server) handle2FAConfirm(w http.ResponseWriter, r *http.Request) {
	id := s.sessionID(r)
	secret, _, ok := pendingSecretLookup(id)
	if !ok {
		s.setFlash(w, r, "totp_setup_expired")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	raw, err := decodeTOTPSecret(secret)
	if err != nil {
		slog.Error("a secret this process generated does not decode", "error", err)
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	// Read before SaveTOTP: once that call succeeds, factorCount is one and the
	// question "was this the first factor" can no longer be asked. An operator
	// who arrived here through the gate was trying to reach the interface, not
	// its settings page — see the JustGated field on passwordPageData for where
	// that earns them a way onward without skipping the recovery codes they
	// have not seen yet.
	wasFirstFactor := s.factorCount() == 0

	// The wide window first, so a right code with a wrong clock gets a diagnosis
	// rather than "wrong code". The fault is on the server; the message must not
	// point at the human.
	_, offset, hit := matchTOTP(raw, time.Now(), r.FormValue("code"), totpWindowEnrol)
	switch {
	case !hit:
		// Marked failed before the re-render regardless of wasFirstFactor: an
		// operator who already has a factor is not locked out by this and
		// simply never uses the card it unlocks — handle2FAEnrolUnverified is the one
		// place that actually decides whether the escape applies, by checking
		// hasSecondFactor() itself. See it for why marking here is unconditional.
		pendingSecretMarkFailed(id)
		s.setFlash(w, r, "totp_code_wrong")
		s.renderSetupAgain(w, r, secret)
		return
	case offset < -totpWindowLogin || offset > totpWindowLogin:
		// Signed magnitude, in whole minutes, rounded the way a human reads it.
		pendingSecretMarkFailed(id)
		s.setFlashN(w, r, clockSkewKey(offset), skewMinutes(offset))
		s.renderSetupAgain(w, r, secret)
		return
	}

	// Eight codes at the first factor, whichever it is, and only at the
	// first: a passkey may already have made this account's mandate satisfied
	// once, and re-pairing TOTP as a second factor — or a fresh secret while
	// it is already the only one — must not silently invalidate a printout
	// the operator already has. hashes defaults to what is already stored, so
	// the SaveTOTP write below leaves it untouched unless this is genuinely
	// the first factor with nothing minted yet.
	var plain []string
	hashes := s.cfg.RecoveryCodes()
	if wasFirstFactor && len(hashes) == 0 {
		p, h, err := newRecoveryCodes()
		if err != nil {
			slog.Error("could not generate recovery codes", "error", err)
			s.setFlash(w, r, "internal_error")
			http.Redirect(w, r, "/password", http.StatusSeeOther)
			return
		}
		plain, hashes = p, h
	}

	if s.client.IsDemo() {
		// The demo runs the whole flow — a real QR code, a real code check, real
		// recovery codes on screen when they are minted — and discards the
		// final write, saying so.
		s.setFlash(w, r, "demo_readonly")
		s.render(w, r, "password.html", "password", s.passwordPage(nil, plain))
		return
	}

	if err := s.cfg.SaveTOTP(secret, hashes); err != nil {
		// Nothing enabled, and the pending secret stays in memory — otherwise the
		// operator re-pairs their app because the disk was briefly full.
		slog.Error("could not store the second factor", "error", err)
		s.setFlash(w, r, "totp_not_saved")
		s.renderSetupAgain(w, r, secret)
		return
	}
	pendingSecretClear(id)

	// The fingerprint just changed, so every other session ends at this moment.
	// Re-stamp our own, exactly as handler_password.go already does after a
	// password change.
	s.restampSession(w, r)
	s.recordLoginEvent(r, shared.EvTOTPEnabled, 0)

	s.setFlash(w, r, "totp_enabled")
	// The codes still have to be shown here — this is the only response that
	// will ever carry them. wasFirstFactor only decides whether the same
	// response also offers a way past the gate they came through.
	page := s.passwordPage(nil, plain)
	page.JustGated = wasFirstFactor
	s.render(w, r, "password.html", "password", page)
}

// handle2FAEnrolUnverified is the third instance of the lockout Ruling 11 found in
// the wizard and round 2 of Task 7 closed there: a clock this page cannot
// fix must never be the only thing standing between an operator and their
// own account. It differs from handleFirstRunRecover in what it writes —
// there, a whole account; here, only the second factor, since the account
// this runs against already exists — so it is its own function rather than
// a shared one forced to take a flag for which write it is doing. See
// finishFirstRunEnrolled's doc comment for the sibling case and why the
// review kept that one shared: the two here would have to be parameterised
// on "is there an account to create," which is exactly the viaOverride bool
// that ruling warned against.
//
// Reachable only when this would be the operator's first factor
// (!hasSecondFactor(), and never in the demo) — an operator who already has
// one is not locked out by a failed code here and can simply leave the
// page, so that case redirects to /password outright rather than rendering
// anything. Past that guard, it needs a code that has already failed
// against this pending entry — the failed pendingSecretLookup returns —
// and an explicit acknowledgement (ack); either one missing re-renders the
// setup card instead of rewarding the request with a stored secret.
func (s *Server) handle2FAEnrolUnverified(w http.ResponseWriter, r *http.Request) {
	if s.hasSecondFactor() || s.client.IsDemo() {
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	id := s.sessionID(r)
	secret, failed, ok := pendingSecretLookup(id)
	if !ok {
		s.setFlash(w, r, "totp_setup_expired")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	if !failed || r.PostFormValue("ack") == "" {
		s.renderSetupAgain(w, r, secret)
		return
	}

	plain, hashes, err := newRecoveryCodes()
	if err != nil {
		slog.Error("could not generate recovery codes", "error", err)
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	if err := s.cfg.SaveTOTP(secret, hashes); err != nil {
		// Same reasoning as handle2FAConfirm's identical branch: the pending
		// entry survives a write failure, so a briefly full disk does not cost
		// the pairing already scanned into the phone.
		slog.Error("could not store the second factor", "error", err)
		s.setFlash(w, r, "totp_not_saved")
		s.renderSetupAgain(w, r, secret)
		return
	}
	pendingSecretClear(id)

	s.restampSession(w, r)
	s.recordLoginEvent(r, shared.EvTOTPEnabled, 0)
	// The only record that will ever explain a stored TOTP secret nobody has
	// verified — see handleFirstRunRecover's identical line. Neither the
	// secret nor the recovery codes belong in a log line.
	slog.Info("second factor enabled via the recovery escape, without a verifying code")

	s.setFlash(w, r, "totp_enabled")
	// This was necessarily the first factor — handle2FAEnrolUnverified refused at the
	// top otherwise — so, exactly as handle2FAConfirm's success path does, the
	// codes shown here also carry the way past the gate the operator arrived
	// through.
	page := s.passwordPage(nil, plain)
	page.JustGated = true
	s.render(w, r, "password.html", "password", page)
}

// renderSetupAgain redraws the setup card with the same secret, so a wrong code
// or a failed write does not cost the operator their pairing.
func (s *Server) renderSetupAgain(w http.ResponseWriter, r *http.Request, secret string) {
	user, _ := s.cfg.Credentials()
	qrURI, err := qrPNGDataURI(otpauthURI(user, secret))
	if err != nil {
		slog.Error("could not render the QR code", "error", err)
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	// The secret is the caller's own, already in hand — only failed is read
	// back here, and pendingSecretLookup happens to be the accessor for both.
	_, failed, _ := pendingSecretLookup(s.sessionID(r))
	s.render(w, r, "password.html", "password", s.passwordPage(&totpSetup{
		// #nosec G203 -- qrURI is "data:image/png;base64," followed by base64 of
		// PNG bytes this process just encoded. Base64 output is [A-Za-z0-9+/=],
		// so no request input can contribute a character that leaves the
		// attribute, whatever the username inside the QR payload happens to be.
		// template.URL is the escaper's own sanctioned bypass; a plain string is
		// silently defanged to #ZgotmplZ and the code never renders.
		QR:         template.URL(qrURI), //nolint:gosec // G203 — see above
		SecretText: formatTOTPSecret(secret),
		ServerTime: time.Now().UTC().Format("2 Jan 2006, 15:04:05 MST"),
		Failed:     failed,
	}, nil))
}

// handle2FADisable switches the factor off.
//
// The current password and no code. Whoever is at this form already holds a
// session that came through the second factor, and a further code would only
// create a new lockout case — phone gone *and* codes gone means the factor
// cannot even be switched off — against no attacker it stops.
func (s *Server) handle2FADisable(w http.ResponseWriter, r *http.Request) {
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
	if err := s.cfg.SaveTOTP("", nil); err != nil {
		slog.Error("could not switch the second factor off", "error", err)
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	s.restampSession(w, r)
	s.recordLoginEvent(r, shared.EvTOTPDisabled, 0)
	s.setFlash(w, r, "totp_disabled")
	http.Redirect(w, r, "/password", http.StatusSeeOther)
}

// handle2FARecovery issues eight fresh codes and invalidates the old ones.
//
// Refuses on a password check alone otherwise: with no factor enrolled there
// is nothing recovery codes are codes *for*, and a gated operator reaching
// this on the strength of their password alone would get eight codes with no
// account state behind them and remain exactly as locked out as before —
// the loophole the review found this route left open once /password could be
// reached with no factor at all. See handle2FAEnrolUnverified for the actual way
// past that gate.
func (s *Server) handle2FARecovery(w http.ResponseWriter, r *http.Request) {
	if !s.hasSecondFactor() && !s.client.IsDemo() {
		s.setFlash(w, r, "totp_recovery_needs_factor")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	if !s.checkCurrentPassword(r) {
		s.setFlash(w, r, "password_wrong")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	plain, hashes, err := newRecoveryCodes()
	if err != nil {
		slog.Error("could not generate recovery codes", "error", err)
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	if s.client.IsDemo() {
		s.setFlash(w, r, "demo_readonly")
		s.render(w, r, "password.html", "password", s.passwordPage(nil, plain))
		return
	}
	if err := s.cfg.SaveRecoveryCodes(hashes); err != nil {
		slog.Error("could not store new recovery codes", "error", err)
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	s.recordLoginEvent(r, shared.EvRecoveryRenewed, 0)
	s.setFlash(w, r, "totp_recovery_renewed")
	s.render(w, r, "password.html", "password", s.passwordPage(nil, plain))
}

// restampSession keeps the acting operator signed in after a fingerprint change.
func (s *Server) restampSession(w http.ResponseWriter, r *http.Request) {
	sess, err := s.store.Get(r, SessionName)
	if err != nil {
		return
	}
	_, hash := s.cfg.Credentials()
	sess.Values[SessionCredentialKey] = credentialFingerprint(hash, s.cfg.TOTPSecret(), s.passkeys.fingerprintInput())
	if err := sess.Save(r, w); err != nil {
		slog.Warn("could not refresh the session after a second-factor change", "error", err)
	}
}

// clockSkewKey picks the message that names which way the clock is wrong, and
// in which grammatical number: skewMinutes(2) is 1, so an offset of 2 steps is
// already the first value that reads "about 1 minute" rather than "about 1
// minutes". Two full messages per direction, chosen by count, is the same
// machinery count_entry_one/count_entry_many already use elsewhere — not a
// second pluralisation mechanism.
func clockSkewKey(offset int) string {
	base := "totp_clock_behind" // the app is ahead of us, so this server is behind
	if offset <= 0 {
		base = "totp_clock_ahead"
	}
	if skewMinutes(offset) == 1 {
		return base + "_one"
	}
	return base + "_many"
}

// skewMinutes turns a step offset into whole minutes, rounded up so "about 1
// minute" never reads as "about 0".
func skewMinutes(offset int) int {
	if offset < 0 {
		offset = -offset
	}
	seconds := offset * int(totpPeriod/time.Second)
	return (seconds + 59) / 60
}
