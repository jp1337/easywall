package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebhookBodyIsJSONWithTheDocumentedFields(t *testing.T) {
	var got map[string]any
	var ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctype = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&got)
	}))
	defer srv.Close()

	n := newNotifier("webhook", srv.URL, "fw1", "v2.20.0")
	if err := n.send(Notification{Event: "rolled_back", Severity: "warning", Detail: "timeout", Time: time.Unix(0, 0).UTC()}); err != nil {
		t.Fatal(err)
	}
	if ctype != "application/json" {
		t.Errorf("content-type = %q", ctype)
	}
	for _, k := range []string{"event", "severity", "detail", "host", "version", "time"} {
		if _, ok := got[k]; !ok {
			t.Errorf("body has no %q: %v", k, got)
		}
	}
}

func TestNtfyBodyIsPlainTextWithHeaders(t *testing.T) {
	var body, ctype, title, prio string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body, ctype, title, prio = string(b), r.Header.Get("Content-Type"), r.Header.Get("Title"), r.Header.Get("Priority")
	}))
	defer srv.Close()

	n := newNotifier("ntfy", srv.URL, "fw1", "v2.20.0")
	if err := n.send(Notification{Event: "panic", Severity: "critical", Detail: "engaged from the console", Time: time.Unix(0, 0).UTC()}); err != nil {
		t.Fatal(err)
	}
	if ctype != "text/plain" {
		t.Errorf("content-type = %q", ctype)
	}
	if !strings.Contains(body, "engaged from the console") {
		t.Errorf("detail is not in the body: %q", body)
	}
	if title == "" || prio == "" {
		t.Errorf("Title=%q Priority=%q — both must be set", title, prio)
	}
}

// Exercised, not read: a test that asserts CheckRedirect is non-nil proves
// nothing about what happens when a server actually redirects.
func TestARedirectIsRefusedRatherThanFollowed(t *testing.T) {
	var reachedSecond bool
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reachedSecond = true
	}))
	defer second.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, second.URL, http.StatusFound)
	}))
	defer first.Close()

	n := newNotifier("webhook", first.URL, "fw1", "v2.20.0")
	err := n.send(Notification{Event: "rolled_back"})
	if err == nil {
		t.Fatal("a redirect was followed silently")
	}
	if reachedSecond {
		t.Fatal("the request reached the redirect target — the destination is not the one configured")
	}
}

func TestTheTimeoutIsEnforced(t *testing.T) {
	old := notifyTimeout
	notifyTimeout = 50 * time.Millisecond
	defer func() { notifyTimeout = old }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	start := time.Now()
	n := newNotifier("webhook", srv.URL, "fw1", "v2.20.0")
	if err := n.send(Notification{Event: "rolled_back"}); err == nil {
		t.Fatal("a hung endpoint returned no error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("send waited %v — the timeout is not bounding it", elapsed)
	}
}

func TestANon2xxIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()
	n := newNotifier("webhook", srv.URL, "fw1", "v2.20.0")
	if err := n.send(Notification{Event: "rolled_back"}); err == nil {
		t.Fatal("a 500 was treated as delivered")
	}
}
