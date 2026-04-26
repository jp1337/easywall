package web

import (
	"net/http"
)

func (s *Server) handleFirstRunGET(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.IsFirstRun() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s.render(w, r, "firstrun.html", "firstrun", nil)
}

func (s *Server) handleFirstRunPOST(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.IsFirstRun() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")

	if username == "" {
		s.setFlash(w, r, "username_required")
		http.Redirect(w, r, "/firstrun", http.StatusSeeOther)
		return
	}
	if len(password) < 12 {
		s.setFlash(w, r, "password_too_short")
		http.Redirect(w, r, "/firstrun", http.StatusSeeOther)
		return
	}
	if password != confirm {
		s.setFlash(w, r, "password_mismatch")
		http.Redirect(w, r, "/firstrun", http.StatusSeeOther)
		return
	}

	hash, err := HashPassword(password)
	if err != nil {
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/firstrun", http.StatusSeeOther)
		return
	}

	if err := s.cfg.SaveCredentials(username, hash); err != nil {
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, "/firstrun", http.StatusSeeOther)
		return
	}

	// First-run complete — redirect to login
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
