package web

import (
	"html/template"
	"log/slog"
	"net/http"
)

// totpSetup is the enrolment card, non-nil only while enrolling.
type totpSetup struct {
	// QR is template.URL rather than string: html/template's contextual
	// escaper treats a plain string in a src attribute as untrusted and
	// refuses any scheme but http/https/mailto, rendering "#ZgotmplZ" instead
	// of the image — silently, with no error, on every page that would have
	// shown a QR code. template.URL is the escaper's own sanctioned way for a
	// caller to certify a value it built itself, never from a request, is safe
	// to place there. qrPNGDataURI's own base64 encoding is what is being
	// certified, not anything a visitor supplied.
	QR         template.URL
	SecretText string // grouped in fours, because it gets copied by hand
	ServerTime string
}

// passwordPageData is what password.html reads.
type passwordPageData struct {
	TOTPEnabled  bool
	RecoveryLeft int
	Setup        *totpSetup
	// Codes is non-nil exactly once, on the response that issues them. There is
	// no route that serves them: a URL that returns the codes would turn "shown
	// once" into "retrievable at any time".
	Codes []string
	Demo  bool
}

func (s *Server) passwordPage(setup *totpSetup, codes []string) passwordPageData {
	return passwordPageData{
		TOTPEnabled:  s.cfg.TOTPEnabled(),
		RecoveryLeft: len(s.cfg.RecoveryCodes()),
		Setup:        setup,
		Codes:        codes,
		Demo:         s.client.IsDemo(),
	}
}

func (s *Server) handlePasswordGET(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "password.html", "password", s.passwordPage(nil, nil))
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
		sess.Values[SessionCredentialKey] = credentialFingerprint(hash, s.cfg.TOTPSecret())
		if err := sess.Save(r, w); err != nil {
			slog.Warn("could not refresh session after password change", "error", err)
		}
	}

	s.setFlash(w, r, "password_changed")
	http.Redirect(w, r, "/password", http.StatusSeeOther)
}
