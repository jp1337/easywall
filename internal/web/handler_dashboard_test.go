package web

import (
	"encoding/json"
	"net/http"
	"os"
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

	// Write a fresh version cache so CheckLatestVersion returns data (covers data.Version = versionInfo)
	info := shared.VersionInfo{
		Current:         "2.0.0",
		Latest:          "2.0.1",
		UpdateAvailable: true,
		ReleaseURL:      "https://example.com",
		CheckedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(info)
	_ = os.WriteFile(s.cfg.VersionCachePath(), data, 0600)

	rec := doAuthRequest(t, s, "GET", "/dashboard", nil)
	assertStatus(t, rec, http.StatusOK)
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
