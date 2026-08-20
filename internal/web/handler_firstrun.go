package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/jp1337/easywall/internal/shared"
)

// firstRunData is what the wizard renders. Defaults are the answers most hosts
// want, so an operator who reads nothing and presses the button still ends up
// somewhere sensible.
//
// It doubles as what a rejected submission comes back as. Everything except the
// two password fields survives being sent back: an operator who mistypes the
// confirmation and retypes only the passwords must not silently re-stage port
// 22 because their answer of 2222 was thrown away.
type firstRunData struct {
	Username  string
	SSHPort   string
	OpenWeb   bool
	IPv6Mode  shared.IPv6Mode
	Telemetry bool

	// WebPort is the port this page is being served on. It is staged as open,
	// and the wizard says so rather than doing it quietly.
	WebPort string
}

// defaultSSHPort is what the wizard offers when the operator has not moved SSH.
const defaultSSHPort = "22"

// firstRunKey holds a rejected submission between the POST and the GET that
// re-renders it. Passwords are never put in it.
const firstRunKey = "firstrun"

func (s *Server) handleFirstRunGET(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.IsFirstRun() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s.render(w, r, "firstrun.html", "firstrun", s.firstRunForm(w, r))
}

// firstRunForm returns the answers to re-display: the ones just rejected if
// there are any, the defaults otherwise.
func (s *Server) firstRunForm(w http.ResponseWriter, r *http.Request) *firstRunData {
	data := &firstRunData{
		SSHPort:  defaultSSHPort,
		IPv6Mode: shared.IPv6Filter,
		WebPort:  s.webPort(),
	}

	sess, _ := s.store.Get(r, SessionName)
	if raw, ok := sess.Values[firstRunKey].(string); ok && raw != "" {
		delete(sess.Values, firstRunKey)
		_ = sess.Save(r, w)
		var prev firstRunData
		if err := json.Unmarshal([]byte(raw), &prev); err == nil {
			prev.WebPort = data.WebPort
			return &prev
		}
	}
	return data
}

// webPort returns the port easywall-web listens on, or "" if bind_addr cannot
// be read as host:port.
func (s *Server) webPort() string {
	_, port, err := net.SplitHostPort(s.cfg.BindAddr)
	if err != nil {
		slog.Warn("cannot read the listening port from bind_addr", "bind_addr", s.cfg.BindAddr, "error", err)
		return ""
	}
	return port
}

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
		s.firstRunError(w, r, "internal_error", nil)
		return
	}

	sshPort := strings.TrimSpace(r.FormValue("ssh_port"))
	if sshPort == "" {
		sshPort = defaultSSHPort
	}
	answers := &firstRunData{
		Username:  strings.TrimSpace(r.FormValue("username")),
		SSHPort:   sshPort,
		OpenWeb:   r.FormValue("open_web") != "",
		IPv6Mode:  ipv6ModeFromForm(r.FormValue("ipv6_mode")),
		Telemetry: r.FormValue("telemetry") != "",
	}

	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")

	switch {
	case answers.Username == "":
		s.firstRunError(w, r, "username_required", answers)
		return
	case len(password) < minPasswordLen:
		s.firstRunError(w, r, "password_too_short", answers)
		return
	case password != confirm:
		s.firstRunError(w, r, "password_mismatch", answers)
		return
	}

	// The port is checked before the account is created: it is the one answer
	// that can lock the operator out of the machine, and it is the one they can
	// still correct while this page is in front of them.
	if _, err := shared.ParsePortNumber(answers.SSHPort); err != nil {
		s.firstRunError(w, r, "firstrun_ssh_port_invalid", answers)
		return
	}

	hash, err := HashPassword(password)
	if err != nil {
		slog.Error("hash password error", "error", err)
		s.firstRunError(w, r, "internal_error", answers)
		return
	}

	if err := s.cfg.SaveFirstRun(FirstRunAccount{
		Username:     answers.Username,
		PasswordHash: hash,
		Telemetry:    answers.Telemetry,
	}); err != nil {
		if errors.Is(err, ErrAlreadySetUp) {
			// Someone else finished the wizard between the check above and this
			// write. There is an account now; it is simply not this one.
			slog.Warn("first run: a second setup arrived after the account existed")
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		slog.Error("save credentials error", "error", err)
		s.firstRunError(w, r, "save_error", answers)
		return
	}

	// From here the account exists and the wizard is closed. The rest is
	// best-effort, and its failures are reported rather than swallowed.
	if err := s.applyFirstRunChoices(answers); err != nil {
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
func (s *Server) applyFirstRunChoices(a *firstRunData) error {
	tcp := []shared.PortRule{{Port: a.SSHPort, Description: "SSH", SSH: true}}

	// The port this page is being served on. Without it the first apply cuts
	// off the interface the operator is standing in: the input policy is drop,
	// nothing opens 12227 by itself, and the acceptance window then rolls the
	// whole apply back. The operator sees an apply that will not stick and no
	// stated reason for it.
	if p := s.webPort(); p != "" && p != a.SSHPort {
		tcp = append(tcp, shared.PortRule{Port: p, Description: "easywall web interface"})
	}

	if a.OpenWeb {
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
	settings.IPv6.Mode = a.IPv6Mode
	return s.client.SaveSettings(*settings)
}

// firstRunError re-renders the wizard with a message, keeping every answer
// except the passwords.
func (s *Server) firstRunError(w http.ResponseWriter, r *http.Request, flash string, answers *firstRunData) {
	if answers != nil {
		if encoded, err := json.Marshal(answers); err == nil {
			sess, _ := s.store.Get(r, SessionName)
			sess.Values[firstRunKey] = string(encoded)
			_ = sess.Save(r, w)
		}
	}
	s.setFlash(w, r, flash)
	http.Redirect(w, r, "/firstrun", http.StatusSeeOther)
}
