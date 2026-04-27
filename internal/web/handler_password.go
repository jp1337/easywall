package web

import (
	"log/slog"
	"net/http"
)

func (s *Server) handlePasswordGET(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "password.html", "password", nil)
}

func (s *Server) handlePasswordPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	current := r.FormValue("current_password")
	newPw := r.FormValue("new_password")
	confirm := r.FormValue("confirm_password")

	if !VerifyPassword(current, s.cfg.Password) {
		s.setFlash(w, r, "password_wrong")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	if len(newPw) < 12 {
		s.setFlash(w, r, "password_too_short")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	if newPw != confirm {
		s.setFlash(w, r, "password_mismatch")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	hash, err := HashPassword(newPw)
	if err != nil {
		slog.Error("hash password error", "error", err)
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	if err := s.cfg.SaveCredentials(s.cfg.Username, hash); err != nil {
		slog.Error("save credentials error", "error", err)
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "password_changed")
	http.Redirect(w, r, "/password", http.StatusSeeOther)
}
