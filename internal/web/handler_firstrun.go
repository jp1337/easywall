package web

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/jp1337/easywall/internal/shared"
)

// firstRunData is what the wizard renders. Defaults are the answers most hosts
// want, so an operator who reads nothing and presses the button still ends up
// somewhere sensible.
type firstRunData struct {
	SSHPort  string
	IPv6Mode shared.IPv6Mode
}

func (s *Server) handleFirstRunGET(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.IsFirstRun() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s.render(w, r, "firstrun.html", "firstrun", &firstRunData{
		SSHPort:  defaultSSHPort,
		IPv6Mode: shared.IPv6Filter,
	})
}

// defaultSSHPort is what the wizard offers when the operator has not moved SSH.
const defaultSSHPort = "22"

// handleFirstRunPOST creates the account and records the first decisions.
//
// The account is written first and everything else afterwards, because the
// wizard closes the moment a password exists: if the core is unreachable, an
// operator with an account can still get in and set the rest by hand, whereas an
// operator without one cannot get in at all. Whatever did not land is named in
// the message rather than left to be discovered.
func (s *Server) handleFirstRunPOST(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.IsFirstRun() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.firstRunError(w, r, "internal_error")
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")

	switch {
	case username == "":
		s.firstRunError(w, r, "username_required")
		return
	case len(password) < minPasswordLen:
		s.firstRunError(w, r, "password_too_short")
		return
	case password != confirm:
		s.firstRunError(w, r, "password_mismatch")
		return
	}

	// The port is checked before the account is created: it is the one answer
	// that can lock the operator out of the machine, and it is the one they can
	// still correct while this page is in front of them.
	sshPort := strings.TrimSpace(r.FormValue("ssh_port"))
	if sshPort == "" {
		sshPort = defaultSSHPort
	}
	if _, err := shared.ParsePortNumber(sshPort); err != nil {
		s.firstRunError(w, r, "firstrun_ssh_port_invalid")
		return
	}

	mode := ipv6ModeFromForm(r.FormValue("ipv6_mode"))

	hash, err := HashPassword(password)
	if err != nil {
		slog.Error("hash password error", "error", err)
		s.firstRunError(w, r, "internal_error")
		return
	}

	if err := s.cfg.SaveFirstRun(username, hash, r.FormValue("telemetry") != ""); err != nil {
		slog.Error("save credentials error", "error", err)
		s.firstRunError(w, r, "save_error")
		return
	}

	// From here the account exists and the wizard is closed. The rest is
	// best-effort, and its failures are reported rather than swallowed.
	if err := s.applyFirstRunChoices(sshPort, mode, r.FormValue("open_web") != ""); err != nil {
		slog.Warn("first run: could not stage the initial choices", "error", err)
		s.setFlash(w, r, "firstrun_choices_failed")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "firstrun_done")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// applyFirstRunChoices stages the ports and saves the IPv6 mode.
//
// Staged, never applied. easywall's model is that rules go live only through a
// deliberate apply with a window to undo it, and the first run is the worst
// moment to make an exception: nobody has yet checked that they can still reach
// the machine.
func (s *Server) applyFirstRunChoices(sshPort string, mode shared.IPv6Mode, openWeb bool) error {
	tcp := []shared.PortRule{{Port: sshPort, Description: "SSH", SSH: true}}
	if openWeb {
		tcp = append(tcp,
			shared.PortRule{Port: "80", Description: "HTTP"},
			shared.PortRule{Port: "443", Description: "HTTPS"},
		)
	}
	if err := s.client.SaveRules("tcp", tcp); err != nil {
		return err
	}

	// Read before write: SaveSettings replaces the whole section, and answering
	// one question is not a reason to reset the Docker settings with it.
	settings, err := s.client.GetSettings()
	if err != nil {
		return err
	}
	settings.IPv6.Mode = mode
	return s.client.SaveSettings(*settings)
}

// firstRunError re-renders the wizard with a message, keeping what was typed.
func (s *Server) firstRunError(w http.ResponseWriter, r *http.Request, flash string) {
	s.setFlash(w, r, flash)
	http.Redirect(w, r, "/firstrun", http.StatusSeeOther)
}
