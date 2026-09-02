package web

import (
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// An operator who opens /ports during the window must not lose the clock. The
// panic banner is the precedent for something every page carries.
func TestTopbarChip_AppearsOnEveryPageDuringTheWindow(t *testing.T) {
	srv := newDemoTestServer(t)
	openAWindow(t, srv)

	for _, path := range []string{"/dashboard", "/ports", "/log", "/options"} {
		body := getAuthenticated(t, srv, path).Body.String()
		if !strings.Contains(body, `id="apply-chip"`) {
			t.Errorf("%s carries no acceptance chip while a window is open; the operator "+
				"loses the countdown by navigating away from /apply", path)
		}
		if !strings.Contains(body, `data-remaining=`) {
			t.Errorf("%s renders the chip without a remaining value, so it cannot tick", path)
		}
	}
}

// Two clocks side by side make neither of them important, and /apply carries
// one at 40px.
func TestTopbarChip_IsHiddenOnApply(t *testing.T) {
	srv := newDemoTestServer(t)
	openAWindow(t, srv)

	if body := getAuthenticated(t, srv, "/apply").Body.String(); strings.Contains(body, `id="apply-chip"`) {
		t.Error("/apply renders the topbar chip beside its own 40px countdown")
	}
}

func TestTopbarChip_AbsentWhenNoWindowIsOpen(t *testing.T) {
	srv := newDemoTestServer(t)
	if body := getAuthenticated(t, srv, "/dashboard").Body.String(); strings.Contains(body, `id="apply-chip"`) {
		t.Error("the chip is rendered with no window open")
	}
}

// openAWindow stages a change and applies it, leaving the demo's real timer
// running.
func openAWindow(t *testing.T, srv *Server) {
	t.Helper()
	if err := srv.client.SaveRules("tcp", []shared.PortRule{{Port: "8443"}}); err != nil {
		t.Fatalf("SaveRules: %v", err)
	}
	if err := srv.client.ApplyRules(); err != nil {
		t.Fatalf("ApplyRules: %v", err)
	}
}
