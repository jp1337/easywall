package web

import (
	"crypto/subtle"
	"net/http"

	"github.com/gorilla/sessions"
)

func (s *Server) handleLoginGET(w http.ResponseWriter, r *http.Request) {
	if s.cfg.IsFirstRun() {
		http.Redirect(w, r, "/firstrun", http.StatusSeeOther)
		return
	}
	// Already logged in?
	sess, _ := s.store.Get(r, SessionName)
	if sess.Values[SessionUserKey] != nil {
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
	passwordOK := VerifyPassword(password, wantHash)
	usernameOK := subtle.ConstantTimeCompare([]byte(username), []byte(wantUser)) == 1

	if !usernameOK || !passwordOK {
		s.setFlash(w, r, "invalid_credentials")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	sess, _ := s.store.Get(r, SessionName)
	sess.Values[SessionUserKey] = username
	sess.Values[SessionCredentialKey] = credentialFingerprint(wantHash)
	sess.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   SessionLifetime,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	_ = sess.Save(r, w)

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sess, _ := s.store.Get(r, SessionName)
	sess.Options.MaxAge = -1
	_ = sess.Save(r, w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
