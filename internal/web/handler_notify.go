package web

import (
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// notifyData is what the Notifications page renders. Every field is read
// through an accessor rather than off the Config or the Server, for the reason
// each accessor's own comment gives: the poll loop reads what this page writes.
type notifyData struct {
	Kind           string
	URL            string
	OnRolledBack   bool
	OnAccepted     bool
	OnPanic        bool
	OnFailedLogins bool
	LastAt         string
	LastErr        string
}

func (s *Server) handleNotifyGET(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "notify.html", "notify", s.notifyData())
}

func (s *Server) notifyData() *notifyData {
	s.notifyLastMu.Lock()
	at, lastErr := s.notifyLastAt, s.notifyLastErr
	s.notifyLastMu.Unlock()

	// Through the accessors, never the fields. Task 5 put them under
	// c.mu.RLock because this page WRITES those fields while the poll loop
	// reads them — a bare field read here is the race -race found.
	kind, addr := s.cfg.NotifyDestination()
	d := &notifyData{
		Kind: kind, URL: addr,
		OnRolledBack:   s.cfg.NotifyEnabled("rolled_back"),
		OnAccepted:     s.cfg.NotifyEnabled("accepted"),
		OnPanic:        s.cfg.NotifyEnabled("panic"),
		OnFailedLogins: s.cfg.NotifyEnabled("failed_logins"),
		LastErr:        lastErr,
	}
	if !at.IsZero() {
		d.LastAt = at.Format(time.RFC3339)
	}
	return d
}

// validNotifyURL accepts only http and https. Not a private-range block: a
// self-hosted ntfy on the LAN is the normal case for this audience, and the
// operator configuring this owns the host. What is refused is a scheme that
// would make the notifier read a file or dial something that is not a web
// endpoint at all.
func validNotifyURL(raw string) bool {
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func (s *Server) handleNotifyPOST(w http.ResponseWriter, r *http.Request) {
	// The demo shows the form and writes nothing, for the reason
	// handlePasswordPOST gives: web.toml is the one piece of state the in-memory
	// mock does not cover, so SaveNotifications writes the real file. notify_url
	// is a credential — it is in secretManagedKeys and kept at 0600 beside
	// session_key — and without this a visitor to the public demo persists an
	// address of their choosing, which the page then renders back to every other
	// visitor. Enumerated in TestDemoModeRefusesToWriteCredentials, which a new
	// credential-writing route has to join.
	if s.cfg.Demo() {
		s.respondPartialError(w, r, "/notify", "demo_readonly")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.respondPartialError(w, r, "/notify", "save_error")
		return
	}
	kind, raw := r.FormValue("kind"), r.FormValue("url")
	if kind != "" && kind != "webhook" && kind != "ntfy" {
		s.respondPartialError(w, r, "/notify", "notify_kind_invalid")
		return
	}
	if !validNotifyURL(raw) {
		s.respondPartialError(w, r, "/notify", "notify_url_invalid")
		return
	}
	// A destination with no address is notifications configured to fire at
	// nothing: newNotifier refuses to build without both halves, so the four
	// triggers would sit there ticked and silent with the page reporting a
	// successful save. "Nothing" is how you turn this off, and it is the first
	// option in the list.
	if kind != "" && raw == "" {
		s.respondPartialError(w, r, "/notify", "notify_url_required")
		return
	}
	if err := s.cfg.SaveNotifications(kind, raw,
		r.FormValue("on_rolled_back") != "", r.FormValue("on_accepted") != "",
		r.FormValue("on_panic") != "", r.FormValue("on_failed_logins") != ""); err != nil {
		slog.Warn("could not save the notification settings", "error", err)
		s.respondPartialError(w, r, "/notify", "save_error")
		return
	}
	// The live notifier is built from those six fields; without this the new
	// address only takes effect at the next restart.
	s.rebuildNotifier()
	s.respondPartialSave(w, r, "/notify", "notify_saved")
}

// handleNotifyTest sends one notification now, to the address in the form,
// whatever the switches say.
//
// The submitted values, not the stored ones. The button is a type="submit"
// inside the settings form, so htmx posts whatever is typed above it, and both
// the page and docs/_docs/features/notifications.md promise it goes there —
// proving an address before committing it is the point of the button. Reading
// s.currentNotifier() instead sent to the *saved* endpoint while the operator
// watched a "Test sent." toast for the new one they had just typed.
//
// A one-off notifier, and no rebuildNotifier: a test must not change what is
// stored.
//
// Deliberately not routed through dispatchNotification's switch check either:
// the operator pressing this button has asked for exactly one delivery, and a
// test button that stays silent because a trigger is off proves nothing about
// the endpoint.
func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Demo() {
		s.respondPartialError(w, r, "/notify", "notify_demo_no_send")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.respondPartialError(w, r, "/notify", "save_error")
		return
	}
	kind, raw := r.FormValue("kind"), r.FormValue("url")
	// Refused exactly the way the save path refuses them, from the same
	// validNotifyURL: an address the save would not accept is not an address
	// worth dialling, and two different answers for one typo is a worse page.
	if kind != "" && kind != "webhook" && kind != "ntfy" {
		s.respondPartialError(w, r, "/notify", "notify_kind_invalid")
		return
	}
	if !validNotifyURL(raw) {
		s.respondPartialError(w, r, "/notify", "notify_url_invalid")
		return
	}
	// Still "set an address first" — and now it is true of the field the
	// operator is looking at rather than of a file they cannot see.
	if kind == "" || raw == "" {
		s.respondPartialError(w, r, "/notify", "notify_not_configured")
		return
	}
	err := newNotifierFor(kind, raw).send(Notification{
		Event: "test", Severity: "info",
		Detail: "This is a test from easywall. Nothing happened to your firewall.",
		Time:   time.Now().UTC(),
	})
	s.notifyLastMu.Lock()
	s.notifyLastAt, s.notifyLastErr = time.Now(), ""
	if err != nil {
		s.notifyLastErr = err.Error()
	}
	s.notifyLastMu.Unlock()
	if err != nil {
		s.respondPartialError(w, r, "/notify", "notify_test_failed")
		return
	}
	s.respondPartialSave(w, r, "/notify", "notify_test_sent")
}
