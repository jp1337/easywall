package web

import (
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

	if username != s.cfg.Username || !VerifyPassword(password, s.cfg.Password) {
		s.setFlash(w, r, "invalid_credentials")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	sess, _ := s.store.Get(r, SessionName)
	sess.Values[SessionUserKey] = username
	sess.Values[SessionCredentialKey] = credentialFingerprint(s.cfg.Password)
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
