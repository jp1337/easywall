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

func pendingSecretLookup(sessionID string) (string, bool) {
	if sessionID == "" {
		return "", false
	}
	pendingSecrets.mu.Lock()
	defer pendingSecrets.mu.Unlock()
	p, ok := pendingSecrets.at[sessionID]
	if !ok || time.Since(p.issued) > pendingSecretLifetime {
		return "", false
	}
	return p.secret, true
}

func pendingSecretClear(sessionID string) {
	pendingSecrets.mu.Lock()
	defer pendingSecrets.mu.Unlock()
	delete(pendingSecrets.at, sessionID)
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
	secret, ok := pendingSecretLookup(id)
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

	// The wide window first, so a right code with a wrong clock gets a diagnosis
	// rather than "wrong code". The fault is on the server; the message must not
	// point at the human.
	_, offset, hit := matchTOTP(raw, time.Now(), r.FormValue("code"), totpWindowEnrol)
	switch {
	case !hit:
		s.setFlash(w, r, "totp_code_wrong")
		s.renderSetupAgain(w, r, secret)
		return
	case offset < -totpWindowLogin || offset > totpWindowLogin:
		// Signed magnitude, in whole minutes, rounded the way a human reads it.
		s.setFlashN(w, r, clockSkewKey(offset), skewMinutes(offset))
		s.renderSetupAgain(w, r, secret)
		return
	}

	if s.client.IsDemo() {
		// The demo runs the whole flow — a real QR code, a real code check, real
		// recovery codes on screen — and discards the final write, saying so.
		plain, _, err := newRecoveryCodes()
		if err != nil {
			s.setFlash(w, r, "internal_error")
			http.Redirect(w, r, "/password", http.StatusSeeOther)
			return
		}
		s.setFlash(w, r, "demo_readonly")
		s.render(w, r, "password.html", "password", s.passwordPage(nil, plain))
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
	s.render(w, r, "password.html", "password", s.passwordPage(nil, plain))
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
	s.render(w, r, "password.html", "password", s.passwordPage(&totpSetup{
		QR:         template.URL(qrURI), //nolint:gosec // G203 — qrURI is base64 PNG bytes this process just encoded, never request input
		SecretText: formatTOTPSecret(secret),
		ServerTime: time.Now().UTC().Format("2 Jan 2006, 15:04:05 MST"),
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
func (s *Server) handle2FARecovery(w http.ResponseWriter, r *http.Request) {
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
	sess.Values[SessionCredentialKey] = credentialFingerprint(hash, s.cfg.TOTPSecret())
	if err := sess.Save(r, w); err != nil {
		slog.Warn("could not refresh the session after a second-factor change", "error", err)
	}
}

// clockSkewKey picks the message that names which way the clock is wrong.
func clockSkewKey(offset int) string {
	if offset > 0 {
		return "totp_clock_behind" // the app is ahead of us, so this server is behind
	}
	return "totp_clock_ahead"
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
