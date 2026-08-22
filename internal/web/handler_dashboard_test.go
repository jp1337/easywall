package web

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHandleDashboard_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/dashboard", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleDashboard_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{
		Active:     true,
		Acceptance: shared.AcceptanceIdle,
		HasPending: false,
	}))

	rec := doAuthRequest(t, s, "GET", "/dashboard", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleDashboard_CoreUnavailable(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetStatus, errorRespFor("core not reachable"))

	rec := doAuthRequest(t, s, "GET", "/dashboard", nil)
	// Should still render the page (with error info), not a server error
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleDashboard_WithVersionCache(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{Active: true}))

	// A fresh cache naming a release ahead of this build. Fresh means the
	// checker answers from it without a request, so the test needs no network.
	info := shared.VersionInfo{
		Current:         shared.CurrentVersion,
		Latest:          "v99.0.0",
		UpdateAvailable: true,
		ReleaseURL:      "https://example.com/releases/v99.0.0",
		CheckedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(info)
	if err := os.WriteFile(s.cfg.VersionCachePath(), data, 0600); err != nil {
		t.Fatalf("write version cache: %v", err)
	}
	s.version = shared.NewChecker(s.cfg.VersionCachePath(), true)

	rec := doAuthRequest(t, s, "GET", "/dashboard", nil)
	assertStatus(t, rec, http.StatusOK)

	// The old test asserted only the status code, so it passed whether or not
	// the version ever reached the page.
	if body := rec.Body.String(); !strings.Contains(body, "v99.0.0") {
		t.Error("the dashboard must show the newer release it knows about")
	}
}

func TestHandleDashboard_DoesNotWaitOnAnUnreachableUpdateAPI(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{Active: true}))

	// No cache, update check on, and nothing listening: the state of a host
	// with no route out. The dashboard used to sit through the full HTTP
	// timeout here, on every single load.
	s.version = shared.NewChecker(s.cfg.VersionCachePath(), true)

	start := time.Now()
	rec := doAuthRequest(t, s, "GET", "/dashboard", nil)
	elapsed := time.Since(start)

	assertStatus(t, rec, http.StatusOK)
	if elapsed > time.Second {
		t.Errorf("dashboard took %v to render; it must not wait for the update check", elapsed)
	}
}

func TestHandleDashboard_RootRedirectsToDashboard(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doAuthRequest(t, s, "GET", "/", nil)
	assertRedirect(t, rec, "/dashboard")
}

func TestHandleDashboard_WithRuleCounts(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rules := shared.RulesState{
		Current: shared.Rules{
			TCP:       []shared.PortRule{{Port: "22"}, {Port: "80"}},
			UDP:       []shared.PortRule{{Port: "53"}},
			Blacklist: []string{"1.2.3.4"},
			Whitelist: []string{"10.0.0.0/8", "192.168.0.0/16"},
		},
	}
	fc.SetResponse(shared.CmdGetRules, successResp(rules))

	rec := doAuthRequest(t, s, "GET", "/dashboard", nil)
	assertStatus(t, rec, http.StatusOK)
}

// The tiles count rules, not lines.
//
// The three free-text lists keep the operator's comments and the blank lines
// between groups. Every other counter in the interface skips them; the dashboard
// took a plain len(), so the same list read "12 entries" on the front page and
// "7" on its own page — and the demo, which is what people look at first, ships
// lists full of comments.
func TestDashboardCountsIgnoreCommentsAndBlankLines(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Current: shared.Rules{
			Blacklist: []string{"# scanners", "192.0.2.1", "", "192.0.2.2"},
			Whitelist: []string{"# office", "", "203.0.113.10"},
			Custom:    []string{"# note", "tcp dport 9100 accept", ""},
			TCP:       []shared.PortRule{{Port: "22"}},
		},
	}))
	fc.SetResponse(shared.CmdGetLog, successResp([]shared.AuditLogEntry{}))

	rec := doAuthRequest(t, s, "GET", "/dashboard", nil)
	assertStatus(t, rec, http.StatusOK)

	// Counted through the same predicate the editors use, so the two pages agree.
	for _, tc := range []struct {
		name  string
		lines []string
		want  int
	}{
		{"blacklist", []string{"# scanners", "192.0.2.1", "", "192.0.2.2"}, 2},
		{"whitelist", []string{"# office", "", "203.0.113.10"}, 1},
		{"custom", []string{"# note", "tcp dport 9100 accept", ""}, 1},
	} {
		if got := countListEntries(tc.lines); got != tc.want {
			t.Errorf("%s: counted %d, want %d — comments and blanks are not entries",
				tc.name, got, tc.want)
		}
	}
}

func TestDashboard_TheChipCarriesTheCount(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{HasPending: true}))
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Current: shared.Rules{},
		Staged:  shared.Rules{TCP: []shared.PortRule{{Port: "8443"}, {Port: "9443"}}},
	}))
	fc.SetResponse(shared.CmdGetOptions, successResp(shared.FirewallOptions{}))
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))
	fc.SetResponse(shared.CmdGetAppliedConfig, successResp(shared.AppliedConfigResult{}))

	rec := doAuthRequest(t, s, "GET", "/dashboard", nil)
	assertStatus(t, rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), ">2<") {
		t.Errorf("the chip does not carry the count of pending changes:\n%s", rec.Body.String())
	}
}
