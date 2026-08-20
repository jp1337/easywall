package web

import (
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

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

	// WantTOTP is the checkbox. It survives a rejected submission like every
	// other answer: an operator who mistypes the confirmation must not have to
	// remember they had asked for a second factor.
	WantTOTP bool
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
	s.render(w, r, "firstrun.html", "firstrun", &firstRunPage{Form: s.firstRunForm(w, r)})
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
		WantTOTP:  r.FormValue("want_totp") != "",
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

	// With the box ticked, nothing is written yet. The answers and the hash go
	// into memory, the secret is generated, and step 2 is rendered as this
	// POST's own response — not a GET with a URL, so a reload cannot mint a
	// second secret.
	if answers.WantTOTP {
		s.beginFirstRunTOTP(w, r, answers, hash)
		return
	}

	written, staged, saveErr := s.completeFirstRun(w, r, FirstRunAccount{
		Username:     answers.Username,
		PasswordHash: hash,
		Telemetry:    answers.Telemetry,
	}, answers)
	if !written {
		// saveErr is nil when completeFirstRun already answered the request
		// itself — the ErrAlreadySetUp race, identical for every caller. This
		// path has no pairing to protect, so a real write failure keeps its
		// original response: re-render step 1 with the answers kept.
		if saveErr != nil {
			s.firstRunError(w, r, "save_error", answers)
		}
		return
	}
	if !staged {
		s.setFlash(w, r, "firstrun_choices_failed")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "firstrun_done")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// completeFirstRun performs the one write and, best-effort, the staging that
// follows it. written is whether the account now exists; staged is whether
// applyFirstRunChoices also succeeded. A caller with nothing further of its
// own to show (the plain wizard, the skip path) only needs written. The
// confirm path needs both, because it has eight recovery codes that must
// reach the operator regardless of what happened to the ports and the IPv6
// mode — collapsing the two once meant a staging failure silently discarded
// codes that had already been generated and hashed to disk.
//
// A staging failure does not redirect from here for exactly that reason: only
// the caller knows whether it still has something to show before the
// operator is sent to /login.
//
// The account is written first and the choices staged afterwards, because the
// wizard closes the moment a password exists: an operator with an account can
// still get in and set the rest by hand, whereas an operator without one
// cannot get in at all.
//
// The write failure returned as saveErr is deliberately *not* answered from
// here, unlike the ErrAlreadySetUp race just above it. That race ends the
// same way for every caller — the account now belongs to whoever won it, so
// /login is right regardless of which route arrived second. A genuine write
// failure does not: the plain and skip paths have no pairing to protect and
// keep re-rendering step 1, but the confirm path has a secret already shown to
// the operator, and sending it back to step 1 mints a fresh one and orphans
// that pairing. Each caller decides for itself; see handleFirstRunConfirm for
// the one that differs.
func (s *Server) completeFirstRun(w http.ResponseWriter, r *http.Request, a FirstRunAccount, answers *firstRunData) (written, staged bool, saveErr error) {
	if err := s.cfg.SaveFirstRun(a); err != nil {
		if errors.Is(err, ErrAlreadySetUp) {
			slog.Warn("first run: a second setup arrived after the account existed")
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return false, false, nil
		}
		slog.Error("save credentials error", "error", err)
		return false, false, err
	}

	if err := s.applyFirstRunChoices(answers); err != nil {
		slog.Warn("first run: could not stage the initial choices", "error", err)
		return true, false, nil
	}
	return true, true, nil
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

// firstRunSetup is the setup step's page data. Non-nil only on that step.
type firstRunSetup struct {
	QR         template.URL
	SecretText string
	ServerTime string
}

// firstRunPage is what firstrun.html reads once the wizard has more than one
// state. Form is the answers to re-display; Setup and Codes are each non-nil on
// exactly one step.
type firstRunPage struct {
	Form  *firstRunData
	Setup *firstRunSetup
	Codes []string
}

// beginFirstRunTOTP generates a secret and shows it. Nothing is stored.
func (s *Server) beginFirstRunTOTP(w http.ResponseWriter, r *http.Request, answers *firstRunData, hash string) {
	secret, err := newTOTPSecret()
	if err != nil {
		slog.Error("could not generate a TOTP secret", "error", err)
		s.firstRunError(w, r, "internal_error", answers)
		return
	}

	// A fresh random id rather than the session's own: during the first run
	// there is no session id at all. SessionIDKey is only set at login, and the
	// wizard runs before an account — let alone a session — exists. This is why
	// the pending entry is keyed here rather than by s.sessionID(r) the way
	// pendingSecrets in handler_2fa.go keys its entries.
	id := newFirstRunPendingID()
	firstRunPendingStore(id, pendingFirstRun{
		Answers:      *answers,
		PasswordHash: hash,
		Secret:       secret,
		Issued:       time.Now(),
	})

	sess, _ := s.store.Get(r, SessionName)
	sess.Values[firstRunPendingKey] = id
	if err := sess.Save(r, w); err != nil {
		slog.Error("could not record the pending first run", "error", err)
		s.firstRunError(w, r, "internal_error", answers)
		return
	}

	s.renderFirstRunSetup(w, r, id, answers, secret)
}

// renderFirstRunSetup draws the setup step, with the same secret, so a wrong
// code or a failed write does not cost the operator their pairing.
//
// Every render refreshes the pending entry's Issued stamp, so the ten minutes
// it is held for measures inactivity on this step rather than total elapsed
// time since the QR code first appeared — see firstRunPendingRefresh for the
// guard against reviving one that had already timed out.
func (s *Server) renderFirstRunSetup(w http.ResponseWriter, r *http.Request, id string, answers *firstRunData, secret string) {
	firstRunPendingRefresh(id)
	qrURI, err := qrPNGDataURI(otpauthURI(answers.Username, secret))
	if err != nil {
		slog.Error("could not render the QR code", "error", err)
		s.firstRunError(w, r, "internal_error", answers)
		return
	}
	s.render(w, r, "firstrun.html", "firstrun", &firstRunPage{
		Form: answers,
		Setup: &firstRunSetup{
			// #nosec G203 -- qrURI is "data:image/png;base64," followed by base64
			// of PNG bytes this process just encoded; base64 output is
			// [A-Za-z0-9+/=], so nothing from the form can leave the attribute.
			// template.URL is the escaper's sanctioned bypass — a plain string is
			// silently defanged to #ZgotmplZ and the code never renders.
			QR:         template.URL(qrURI), //nolint:gosec // G203 — see above
			SecretText: formatTOTPSecret(secret),
			ServerTime: time.Now().UTC().Format("2 Jan 2006, 15:04:05 MST"),
		},
	})
}

// pendingFirstRunFor returns the half-finished setup this request carries.
func (s *Server) pendingFirstRunFor(r *http.Request) (string, pendingFirstRun, bool) {
	sess, err := s.store.Get(r, SessionName)
	if err != nil {
		return "", pendingFirstRun{}, false
	}
	id, _ := sess.Values[firstRunPendingKey].(string)
	p, ok := firstRunPendingLookup(id)
	return id, p, ok
}

// firstRunExpired answers a request whose pending entry did not check out —
// gone entirely, or aged past firstRunPendingLifetime. Ten minutes is not
// generous for "install an authenticator app, scan, mistype twice, read the
// server time", and the escape hatch exists precisely so a clock cannot cost
// an account; it must not itself dead-end into a blank wizard with no
// explanation.
//
// The same totp_setup_expired handle2FAConfirm already uses for the
// equivalent case on the password page. The answers are restored when there
// are any to restore: firstRunPendingLookup returns a populated-but-expired
// entry when one exists, and the zero value when there was never anything to
// find, and stashing that zero value would blank the form's defaults rather
// than leave them alone.
func (s *Server) firstRunExpired(w http.ResponseWriter, r *http.Request, p pendingFirstRun) {
	var stash *firstRunData
	if !p.Issued.IsZero() {
		stash = &p.Answers
	}
	s.firstRunError(w, r, "totp_setup_expired", stash)
}

// handleFirstRunConfirm checks the code and, only then, writes.
func (s *Server) handleFirstRunConfirm(w http.ResponseWriter, r *http.Request) {
	id, p, ok := s.pendingFirstRunFor(r)
	if !ok {
		s.firstRunExpired(w, r, p)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.firstRunError(w, r, "internal_error", &p.Answers)
		return
	}

	raw, err := decodeTOTPSecret(p.Secret)
	if err != nil {
		slog.Error("a secret this process generated does not decode", "error", err)
		s.firstRunError(w, r, "internal_error", &p.Answers)
		return
	}

	// The wide window first, so a right code against a wrong clock is told the
	// truth rather than "wrong code".
	_, offset, hit := matchTOTP(raw, time.Now(), strings.TrimSpace(r.FormValue("code")), totpWindowEnrol)
	switch {
	case !hit:
		// firstrun_totp_code_wrong, not the shared totp_code_wrong: this is the
		// only page where a code that will never verify — a board with no RTC,
		// still at the epoch — must not be a dead end. /password's identical
		// wrong-code case has an account already and stays on the plain message.
		s.setFlash(w, r, "firstrun_totp_code_wrong")
		s.renderFirstRunSetup(w, r, id, &p.Answers, p.Secret)
		return
	case offset < -totpWindowLogin || offset > totpWindowLogin:
		s.setFlashN(w, r, clockSkewKey(offset), skewMinutes(offset))
		s.renderFirstRunSetup(w, r, id, &p.Answers, p.Secret)
		return
	}

	plain, hashes, err := newRecoveryCodes()
	if err != nil {
		slog.Error("could not generate recovery codes", "error", err)
		s.firstRunError(w, r, "internal_error", &p.Answers)
		return
	}

	written, staged, saveErr := s.completeFirstRun(w, r, FirstRunAccount{
		Username:       p.Answers.Username,
		PasswordHash:   p.PasswordHash,
		Telemetry:      p.Answers.Telemetry,
		TOTPSecret:     p.Secret,
		RecoveryHashes: hashes,
	}, &p.Answers)
	if !written {
		// The entry deliberately survives a failed write: otherwise the operator
		// retypes a password and re-pairs a phone because a disk was briefly full.
		//
		// saveErr is nil on the ErrAlreadySetUp race, which completeFirstRun has
		// already answered with a redirect to /login — nothing more to do here.
		// A real write failure gets a response of its own: the same secret shown
		// again, with the message that names what actually failed. Answering with
		// save_error's redirect to /firstrun instead — the shape this used to
		// take — would strand the pending entry: step 1 has no way back to step 2,
		// and the retyped submission that follows mints a fresh secret, orphaning
		// the pairing the operator just made.
		if saveErr != nil {
			s.setFlash(w, r, "totp_not_saved")
			s.renderFirstRunSetup(w, r, id, &p.Answers, p.Secret)
		}
		return
	}
	firstRunPendingClear(id)

	// The codes reach the operator whatever happened to staged: they were
	// generated and hashed into the account the moment it was written, and
	// there is no second chance to see them. A ports/IPv6 failure here must
	// not cost the operator their only look at a second factor's recovery
	// codes — see completeFirstRun.
	if staged {
		s.setFlash(w, r, "firstrun_done")
	} else {
		s.setFlash(w, r, "firstrun_done_choices_failed")
	}
	s.render(w, r, "firstrun.html", "firstrun", &firstRunPage{Form: &p.Answers, Codes: plain})
}

// handleFirstRunSkip creates the account without a factor.
//
// This is the branch that keeps an optional feature from becoming a way of not
// getting an account. easywall runs on boards with no RTC, which come up at the
// epoch until NTP lands; if a correct code were the only way past the setup step,
// a flat battery would mean no account at all on a machine already reachable from
// the network. It takes today's path exactly.
func (s *Server) handleFirstRunSkip(w http.ResponseWriter, r *http.Request) {
	id, p, ok := s.pendingFirstRunFor(r)
	if !ok {
		s.firstRunExpired(w, r, p)
		return
	}
	written, staged, saveErr := s.completeFirstRun(w, r, FirstRunAccount{
		Username:     p.Answers.Username,
		PasswordHash: p.PasswordHash,
		Telemetry:    p.Answers.Telemetry,
	}, &p.Answers)
	if !written {
		// No pairing at stake on this path — see completeFirstRun — so a real
		// write failure keeps its original response: back to step 1 with the
		// answers kept, same as the plain wizard's.
		if saveErr != nil {
			s.firstRunError(w, r, "save_error", &p.Answers)
		}
		return
	}
	firstRunPendingClear(id)

	if !staged {
		s.setFlash(w, r, "firstrun_choices_failed")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "firstrun_done")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
