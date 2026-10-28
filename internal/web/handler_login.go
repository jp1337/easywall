package web

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/sessions"

	"github.com/jp1337/easywall/internal/shared"
)

func (s *Server) handleLoginGET(w http.ResponseWriter, r *http.Request) {
	if s.cfg.IsFirstRun() {
		http.Redirect(w, r, "/firstrun", http.StatusSeeOther)
		return
	}
	// Already logged in? The same test RequireAuth applies, or the two disagree
	// about a revoked session and bounce the browser between them.
	sess, _ := s.store.Get(r, SessionName)
	if sessionUser(sess, s.currentCredential()) != "" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	s.render(w, r, "login.html", "login", nil)
}

func (s *Server) handleLoginPOST(w http.ResponseWriter, r *http.Request) {
	if s.cfg.IsFirstRun() {
		http.Redirect(w, r, "/firstrun", http.StatusSeeOther)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	// Both checks always run, and neither short-circuits the other.
	//
	// `username != wantUser || !VerifyPassword(...)` skipped the argon2
	// verification whenever the name was wrong, and argon2 is deliberately slow:
	// a wrong username answered in 60µs and the right one in 37ms, a 600-fold
	// difference readable over any network. That is not a side channel so much
	// as a lookup — one request per guess tells an attacker the account name of
	// the single account this system has.
	wantUser, wantHash := s.cfg.Credentials()
	ctx, cancel := context.WithTimeout(r.Context(), loginHashWait)
	passwordOK, err := verifyPasswordCtx(ctx, password, wantHash)
	cancel()
	if err != nil {
		// Not checked at all, so neither a success nor a failure to record.
		w.Header().Set("Retry-After", "5")
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	usernameOK := subtle.ConstantTimeCompare([]byte(username), []byte(wantUser)) == 1

	if !usernameOK || !passwordOK {
		s.recordLoginEvent(r, shared.EvLoginFailed, 0)
		s.setFlash(w, r, "invalid_credentials")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// With no second factor at all — no TOTP secret and no enrolled passkey —
	// this is the whole login and nothing has changed. Checked through
	// hasSecondFactor(), not TOTPSecret() alone: an account whose only factor
	// is a passkey must reach the second step exactly as a TOTP-only account
	// does, or the passkey it enrolled is never asked for at login and the
	// mandate is satisfied in name only.
	if !s.hasSecondFactor() {
		s.grantSession(w, r, username)
		s.recordLoginEvent(r, shared.EvLoginOK, 0)
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// With one, the password step ends here: an intermediate state and a
	// redirect, and no session of any kind. Nothing sessionUser() reads is
	// written, which is what makes it impossible for the two to disagree about
	// whether this visitor is signed in — see the comment above sessionUser.
	pending := pendingLogin{
		User:     username,
		CredFP:   credentialFingerprint(wantHash, s.cfg.TOTPSecret(), s.passkeys.fingerprintInput()),
		IssuedAt: time.Now().Unix(),
	}
	if err := s.writePending(w, r, pending); err != nil {
		slog.Error("could not begin the second step", "error", err)
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/login/verify", http.StatusSeeOther)
}

// handleLoginDemo signs a visitor into the demo without a password.
//
// Registered only when demo_mode is set, so on an installation there is no
// route here at all. Nothing behind a demo session reaches a firewall — the
// client is the in-memory mock and every credential write is refused — so a
// password guarded nothing, and typing one kept every browser review waiting
// on a human. The check below is the second lock on a door that is not built
// outside the demo.
func (s *Server) handleLoginDemo(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Demo() {
		http.NotFound(w, r)
		return
	}
	username, _ := s.cfg.Credentials()
	s.grantSession(w, r, username)
	s.recordLoginEvent(r, shared.EvLoginOK, 0)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// grantSession issues the session cookie a completed login earns.
//
// One place, because both the one-step and the two-step login end here and a
// session issued two ways is a session stamped two ways.
func (s *Server) grantSession(w http.ResponseWriter, r *http.Request, username string) {
	_, hash := s.cfg.Credentials()
	sess, _ := s.store.Get(r, SessionName)
	sess.Values[SessionUserKey] = username
	sess.Values[SessionCredentialKey] = credentialFingerprint(hash, s.cfg.TOTPSecret(), s.passkeys.fingerprintInput())
	sess.Values[SessionIDKey] = newSessionID()
	sess.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   SessionLifetime,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	_ = sess.Save(r, w)
}

// loginVerifyPage is the data behind login_verify.html: whether to draw the
// passkey button above the code field at all.
type loginVerifyPage struct {
	ShowPasskey bool
}

func (s *Server) handleLoginVerifyGET(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.pendingForRequest(r); !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	// Not instead of the code field — an operator whose phone is flat still
	// needs it — and not drawn at all when there is nothing to offer: neither
	// passkeyUnavailableReason's own three checks nor an empty enrolled set
	// leaves an operator mid-login with a control that can only fail.
	data := loginVerifyPage{
		ShowPasskey: s.passkeyUnavailableReason() == "" && len(s.passkeys.all()) > 0,
	}
	s.render(w, r, "login_verify.html", "login", data)
}

// pendingForRequest returns the intermediate state, refusing one that no longer
// matches the credentials in force. A password change ends half-logins too.
func (s *Server) pendingForRequest(r *http.Request) (pendingLogin, bool) {
	p, ok := s.readPending(r)
	if !ok {
		return pendingLogin{}, false
	}
	_, hash := s.cfg.Credentials()
	if p.CredFP != credentialFingerprint(hash, s.cfg.TOTPSecret(), s.passkeys.fingerprintInput()) {
		return pendingLogin{}, false
	}
	return p, true
}

// spendPendingAttempt is pendingForRequest for the two doors that check a
// factor: it also reserves the attempt the check is about to spend, so the
// budget is taken before the guess is looked at rather than charged after it.
// See reservePendingAttempt for what the charge-after order allowed.
func (s *Server) spendPendingAttempt(r *http.Request) (pendingLogin, bool) {
	p, ok := s.pendingForRequest(r)
	if !ok {
		return pendingLogin{}, false
	}
	if p.Attempts, ok = reservePendingAttempt(p.ID); !ok {
		return pendingLogin{}, false
	}
	return p, true
}

// handleLoginVerifyPOST is the second step.
//
// It has no IP limit of its own, and the arithmetic is why: one intermediate
// state allows pendingMaxAttempts code attempts; a new intermediate state costs
// one password round; those are capped at 5 per 10 minutes per address. That is
// 15 code attempts per 10 minutes per address against a target that rotates
// every 30 seconds, and without a valid cookie this route is a redirect that
// costs nothing.
//
// That arithmetic was false when it was written, and the correction is the
// point of pendingattempts.go. The inner budget lived only in the cookie the
// server handed back, so it bound a browser and not an attacker: one password
// round, one frozen cookie, replayed — 200 guesses measured where 3 were
// claimed, at a rate that covers roughly 9.5M of the 10^6 code space inside one
// 180-second window. The count is now server-side, keyed by an id the cookie
// carries, and readPending enforces it before any caller sees a pendingLogin.
//
// Three kinds of test hold the three halves apart, because none of them proves
// another. TestLoginVerify_ThreeWrongCodesEndTheAttempt and
// TestThreePasskeyFailuresEndTheAttempt prove pendingMaxAttempts for a
// cooperating browser — the inner budget the code field and the passkey button
// share. TestLoginVerify_TheSixteenthCodeAttemptDoesNotGetThrough and
// TestTheSixteenthPasskeyAttemptDoesNotGetThrough take a fresh intermediate
// state per round, so what they prove is the outer bound of 15.
// TestPending_AFrozenCookieDoesNotBuyMoreAttempts and its passkey twin present
// one unchanging cookie, which is the only shape that proves the inner budget
// is the server's and not the client's.
func (s *Server) handleLoginVerifyPOST(w http.ResponseWriter, r *http.Request) {
	p, ok := s.spendPendingAttempt(r)
	if !ok {
		s.clearPending(w, r)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	code := strings.TrimSpace(r.FormValue("code"))

	// One field; the shape decides. isTOTPShape and isRecoveryShape are disjoint
	// by construction — six digits against ten Crockford symbols — which is the
	// reason the recovery format is what it is.
	switch {
	case isTOTPShape(code) && s.acceptTOTP(code):
		s.clearPending(w, r)
		s.grantSession(w, r, p.User)
		s.recordLoginEvent(r, shared.EvLoginOK, 0)
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return

	case isRecoveryShape(code):
		hashes := s.cfg.RecoveryCodes()
		i := matchRecoveryCode(code, hashes)
		if i < 0 {
			break
		}
		// Removed by hash under the config lock, not by writing back the set
		// read above — see RemoveRecoveryCode for the double spend that allowed.
		// Not removed means another request spent it first: a wrong code.
		left, removed, err := s.cfg.RemoveRecoveryCode(hashes[i])
		if !removed {
			break
		}
		// The session is granted even if the write fails: the code is in the
		// operator's hands, not an attacker's, and "a full disk locks you out of
		// your firewall" is the worse outcome — the same weighing as "refusing to
		// restore is worse" in 2.7.
		if err != nil {
			slog.Error("a recovery code was accepted but could not be consumed; it will "+
				"work again until this is fixed", "error", err, "remaining", left)
			s.setFlashN(w, r, "recovery_not_consumed", left)
		} else {
			s.setFlashN(w, r, "recovery_left", left)
		}
		s.clearPending(w, r)
		s.grantSession(w, r, p.User)
		s.recordLoginEvent(r, shared.EvRecoveryUsed, left)
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// Everything else, including a six-digit code that did not match and a
	// recovery-shaped value that is not one of the eight.
	s.failVerifyAttempt(w, r, p, shared.Ev2FAFailed)
}

// failVerifyAttempt is the second step's one budget, shared by both doors: a
// wrong code here and a failed passkey assertion in handleLoginPasskeyFinish
// both charge the same intermediate state, or the passkey route would be a way
// past the limit the code field is subject to. See handleLoginVerifyPOST's own
// comment for the arithmetic this produces.
//
// The charge was already taken by spendPendingAttempt, in the server's map and
// before the check; p.Attempts is that charge. Nothing is written back to the
// cookie, so there is nothing for a client to decline to keep.
func (s *Server) failVerifyAttempt(w http.ResponseWriter, r *http.Request, p pendingLogin, ev shared.LoginEvent) {
	s.recordLoginEvent(r, ev, 0)
	if p.Attempts >= pendingMaxAttempts {
		// Back to /login without saying whether the password or the factor
		// failed, and without ever saying how many attempts were left.
		s.clearPending(w, r)
		s.setFlash(w, r, "login_again")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s.setFlash(w, r, "verify_failed")
	http.Redirect(w, r, "/login/verify", http.StatusSeeOther)
}

// acceptTOTP checks the code against the enrolled secret and the replay store.
//
// Both halves, and the order matters: the step that *matched* is what is stored,
// never the current one. Storing the current step would burn the still-valid
// step N whenever a code from N-1 was accepted, locking the operator out for
// thirty seconds immediately after a successful login.
func (s *Server) acceptTOTP(code string) bool {
	secret := s.cfg.TOTPSecret()
	if secret == "" {
		// A passkey-only account: no secret, so a code is simply wrong. This
		// used to fall through to the decode below, which fails on "" and logged
		// at ERROR that the secret was unusable — telling the operator to delete
		// passkeys.json, the one factor the account has, on every mistyped code.
		return false
	}
	raw, err := decodeTOTPSecret(secret)
	if err != nil {
		slog.Error("the stored TOTP secret cannot be used, so no code can match it; to sign in "+
			"with the password alone, clear totp_secret and recovery_codes in web.toml and "+
			"delete passkeys.json in data_dir", "reason", err)
		return false
	}
	step, _, ok := matchTOTP(raw, time.Now(), code, totpWindowLogin)
	if !ok {
		return false
	}
	return s.replay.accept(step)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sess, _ := s.store.Get(r, SessionName)

	// Telling the browser to drop the cookie is not the same as ending the
	// session. The cookie is a signed, self-contained token: whoever still has
	// the value stayed signed in for the rest of its lifetime, however firmly
	// the button said otherwise. Record it as revoked so it stops working now.
	//
	// The audit event is recorded inside this same guard, not after it. This
	// route is public — POST /logout carries no session requirement, by design,
	// so a signed-out browser can still reach it — and a request with no
	// session id behind it is not a logout, it is a POST from a stranger who
	// was never signed in. Recording it anyway let an unauthenticated,
	// unthrottled request write an audit line on demand; a loop of them erased
	// the visible log (GET_LOG returns only the last 200 lines) in under two
	// seconds.
	//
	// And only for a session that is still signed in. The guard was id != ""
	// alone, and a cookie that was already logged out still carries its id: 300
	// replays of one revoked cookie wrote 301 EvLogout lines, the same erasure by
	// anyone who had held a session once. sessionUser is RequireAuth's own test,
	// so a revoked or superseded session is nobody here too. Revoking is still
	// unconditional on the id — it is idempotent and costs nothing to repeat.
	if id, _ := sess.Values[SessionIDKey].(string); id != "" {
		if sessionUser(sess, s.currentCredential()) != "" {
			s.recordLoginEvent(r, shared.EvLogout, 0)
		}
		revokeSession(id)
	}

	sess.Options.MaxAge = -1
	_ = sess.Save(r, w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
