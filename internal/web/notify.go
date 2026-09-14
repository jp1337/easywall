package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// notifyTimeout bounds one delivery. A var so a test can prove the bound
// exists without waiting for it — shared.Reporter's telemetryTimeout is the
// same shape for the same reason.
var notifyTimeout = 10 * time.Second

// Notification is one thing worth telling an operator about.
type Notification struct {
	Event    string // "rolled_back" | "accepted" | "panic" | "failed_logins"
	Severity string // "info" | "warning" | "critical"
	Detail   string // free text: the acceptance reason, the address, the count
	Time     time.Time
}

// notifier posts a Notification to one operator-chosen destination.
//
// Two body shapes behind one client, not an interface with one implementation
// per transport: ntfy differs from a webhook in its body and three headers,
// which is a switch, not an abstraction.
type notifier struct {
	kind    string
	url     string
	host    string
	version string
	client  *http.Client
}

func newNotifier(kind, url, host, version string) *notifier {
	return &notifier{
		kind: kind, url: url, host: host, version: version,
		client: &http.Client{
			Timeout: notifyTimeout,
			// Refused, not followed. shared.Reporter's send() says why: a
			// destination that follows a redirect can be pointed at a third
			// party by whoever controls its DNS. That reasoning is stronger
			// here — the body describes this operator's firewall.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("the notification endpoint redirected")
			},
		},
	}
}

func (n *notifier) send(msg Notification) error {
	if n.kind == "" || n.url == "" {
		return nil
	}
	req, err := n.build(msg)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "easywall/"+n.version)

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // the status is what matters
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notification endpoint returned %s", resp.Status)
	}
	return nil
}

func (n *notifier) build(msg Notification) (*http.Request, error) {
	if n.kind == "ntfy" {
		req, err := http.NewRequest(http.MethodPost, n.url, bytes.NewBufferString(msg.Detail))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("Title", n.host+": "+msg.Event)
		req.Header.Set("Priority", ntfyPriority(msg.Severity))
		req.Header.Set("Tags", msg.Event)
		return req, nil
	}

	body, err := json.Marshal(map[string]any{
		"event":    msg.Event,
		"severity": msg.Severity,
		"detail":   msg.Detail,
		"host":     n.host,
		"version":  n.version,
		"time":     msg.Time.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// ntfyPriority maps easywall's three severities onto ntfy's five levels. Not
// one-to-one on purpose: easywall has nothing to say at ntfy's "min", and
// "max" is reserved for the one event that means the firewall is down.
func ntfyPriority(severity string) string {
	switch severity {
	case "critical":
		return "5"
	case "warning":
		return "4"
	default:
		return "3"
	}
}

// notifyTick is how often the notifier asks for the firewall's state.
//
// Fifteen seconds. It goes through statusForRender() like a page render does,
// but the cache TTL is 2 seconds and this tick is 15, so every tick misses and
// pays its own GET_STATUS round trip — one to the daemon on this host, none to
// the network. An earlier version of this comment, and of the spec it came
// from, said it "adds no socket traffic of its own". That is false for any tick
// longer than the TTL, and it had propagated as far as the published page
// before a review caught the arithmetic.
//
// It does not need to be faster: accepted and rolled_back are terminal states
// that persist, so a tick that lands after the acceptance window closed still
// finds the outcome.
//
// A var, not a const, so a test can drive the loop without waiting fifteen
// seconds per transition — the same reason notifyTimeout is one.
var notifyTick = 15 * time.Second

// rebuildNotifier builds the client from the configuration in force, or clears
// it. Called at construction and again after a settings change, so a new
// address takes effect without a restart.
//
// nil in demo mode whatever is configured: the public demo is a page anyone on
// the internet can open, and it makes no outbound request.
func (s *Server) rebuildNotifier() {
	kind, url := s.cfg.NotifyDestination()
	var n *notifier
	if !s.cfg.DemoMode && kind != "" && url != "" {
		host, _ := os.Hostname()
		n = newNotifier(kind, url, host, shared.CurrentVersion)
	}
	s.notifyMu.Lock()
	s.notify = n
	s.notifyMu.Unlock()
}

// currentNotifier returns the notifier in force, or nil. The one way to read
// s.notify: the settings page can replace it while this goroutine is posting.
func (s *Server) currentNotifier() *notifier {
	s.notifyMu.RLock()
	defer s.notifyMu.RUnlock()
	return s.notify
}

// runNotifier polls the firewall's state and posts what the operator asked for.
func (s *Server) runNotifier(stop <-chan struct{}) {
	ticker := time.NewTicker(notifyTick)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			// statusForRender() returns nil when the core cannot be reached, and
			// observe already reads nil as "unknown". Passed straight through:
			// a second guard here would make one of the two dead.
			for _, n := range s.notifySeen.observe(s.statusForRender()) {
				s.dispatchNotification(n)
			}
		}
	}
}

// notifyEnabledFor reports whether the operator asked to hear about this event.
// The switch itself is on Config, under the lock that guards the six fields.
func (s *Server) notifyEnabledFor(event string) bool { return s.cfg.NotifyEnabled(event) }

// notifyLoginFailure is auditEvents' burst hook. It sees the address the request
// actually came from — Record calls it before demo mode blanks the recorded one.
//
// The delivery goes to its own goroutine because this runs on the login path,
// and a sign-in must not wait out a 10-second POST to somebody's webhook.
func (s *Server) notifyLoginFailure(ev shared.LoginEvent, addr string) {
	if n := s.notifyBurst.record(ev, addr, time.Now()); n != nil {
		go s.dispatchNotification(*n)
	}
}

// dispatchNotification sends one notification, if it is switched on, and records
// what happened for the page to show.
//
// One retry and no more. A queue is not promised — see the spec's section 8 —
// and a retry loop against an endpoint that is down is a way to be the problem.
func (s *Server) dispatchNotification(n Notification) {
	notify := s.currentNotifier()
	if notify == nil || !s.notifyEnabledFor(n.Event) {
		return
	}
	err := notify.send(n)
	if err != nil {
		err = notify.send(n) // the one retry
	}
	s.notifyLastMu.Lock()
	s.notifyLastAt = time.Now()
	s.notifyLastErr = ""
	if err != nil {
		s.notifyLastErr = err.Error()
	}
	s.notifyLastMu.Unlock()
	if err != nil {
		slog.Warn("could not deliver a notification; it is not queued and will not be retried again",
			"event", n.Event, "error", err)
	}
}
