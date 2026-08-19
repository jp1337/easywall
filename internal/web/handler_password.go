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

	// The demo shows the form and writes nothing. There is nothing to
	// demonstrate past the form here — the interesting half of changing a
	// password is that it is stored — and web.toml is the one piece of state the
	// in-memory mock does not cover: SaveCredentials writes the real file, so a
	// visitor could lock every other visitor out until the process restarted.
	// Enumerated in TestDemoModeRefusesToWriteCredentials, which a new
	// credential-writing route has to join.
	if s.client.IsDemo() {
		s.setFlash(w, r, "demo_readonly")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	current := r.FormValue("current_password")
	newPw := r.FormValue("new_password")
	confirm := r.FormValue("confirm_password")

	username, currentHash := s.cfg.Credentials()
	if !VerifyPassword(current, currentHash) {
		s.setFlash(w, r, "password_wrong")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	if len(newPw) < minPasswordLen {
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

	if err := s.cfg.SaveCredentials(username, hash); err != nil {
		slog.Error("save credentials error", "error", err)
		s.setFlash(w, r, "internal_error")
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}

	// Every session issued under the old password is now invalid. Re-stamp this
	// one so the operator who just changed it is not thrown out of the tab they
	// are working in — anyone else signed in is.
	if sess, err := s.store.Get(r, SessionName); err == nil {
		sess.Values[SessionCredentialKey] = credentialFingerprint(hash)
		if err := sess.Save(r, w); err != nil {
			slog.Warn("could not refresh session after password change", "error", err)
		}
	}

	s.setFlash(w, r, "password_changed")
	http.Redirect(w, r, "/password", http.StatusSeeOther)
}
