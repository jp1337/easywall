package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
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
