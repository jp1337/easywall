package web

import (
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// dashboardWithHealth renders /dashboard against a core answering with one
// health result, and returns the body.
//
// GET_STATUS has to be answered too: the hero is inside `{{if .Status}}`, so a
// test that set only the health reply would assert against a page on which the
// panel does not exist at all — and would pass whatever the health rendering
// did, because the strings it looks for would be absent for the wrong reason.
func dashboardWithHealth(t *testing.T, res shared.HealthResult) string {
	t.Helper()
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{
		Active:     true,
		Acceptance: shared.AcceptanceIdle,
	}))
	fc.SetResponse(shared.CmdGetHealth, successResp(res))

	rec := doAuthRequest(t, s, "GET", "/dashboard", nil)
	if rec.Code != 200 {
		t.Fatalf("GET /dashboard = %d, want 200", rec.Code)
	}
	return rec.Body.String()
}

// A degraded firewall says why, in words.
//
// Not the dot. DESIGN.md's Status section requires both forms simultaneously
// because colour alone fails a colour-blind operator and fails again in a
// screenshot pasted into a ticket, and this release's whole subject is a
// surface that reported "active" while the stateful half enforced nothing. A
// coloured dot with no sentence beside it is that surface again in amber.
//
// Anchored on the sentence's own words rather than on a substring like
// "degraded": the state word and the reason id both contain it, so a
// `Contains(body, "degraded")` would stay true with the reason removed
// entirely — the trap that let a bare Contains(usage, "panic") survive the
// deletion of the panic line earlier in this release.
func TestTheDashboardGivesTheDegradedReasonInWords(t *testing.T) {
	body := dashboardWithHealth(t, shared.HealthResult{
		State:  shared.HealthDegraded,
		Reason: shared.HealthReasonStatefulDead,
		Selftest: shared.HealthSelftest{
			Version: shared.CurrentVersion,
			Kernel:  "6.11.0-easywall",
			Result:  shared.SelftestPassed,
			At:      time.Now(),
		},
	})

	// The reason, as English prose from the locale file.
	if !strings.Contains(body, "the established/related rule is matching none") {
		t.Error("the dashboard does not say why the firewall is degraded; the reason has to " +
			"reach the page as a sentence, not as a colour")
	}
	// The state word beside the dot.
	if !strings.Contains(body, ">Degraded<") {
		t.Error("the hero does not carry the word Degraded beside its dot")
	}
	// And the dot's tone.
	if !strings.Contains(body, `class="hero-dot warn"`) {
		t.Error("a degraded state is not amber; colour means state (DESIGN.md rule 1)")
	}
	if strings.Contains(body, "hero-dot ok") {
		t.Error("a degraded state still renders the green dot")
	}
}

// fail is red and says the same thing about itself.
func TestTheDashboardRendersAFailingFirewallAsCritical(t *testing.T) {
	body := dashboardWithHealth(t, shared.HealthResult{
		State:  shared.HealthFail,
		Reason: shared.HealthReasonNotEnforcing,
	})

	if !strings.Contains(body, `class="hero-dot crit"`) {
		t.Error("a failing firewall is not red")
	}
	if !strings.Contains(body, "so nothing is being filtered") {
		t.Error("the dashboard does not say that nothing is being filtered")
	}
}

// ok says so too, and the sentence it says it with is the health reason rather
// than the old fixed claim.
//
// dashboard_hero_active — "The core daemon is running and rules are live" — is
// the sentence Enforcing()'s doc comment records as having been derived from
// nothing but the daemon being up. It survives as the fallback for a core too
// old to answer GET_HEALTH, and this asserts that a core which *can* answer is
// not read through it.
func TestTheDashboardPrefersTheHealthReasonOverTheFixedClaim(t *testing.T) {
	body := dashboardWithHealth(t, shared.HealthResult{
		State:  shared.HealthOK,
		Reason: shared.HealthReasonHealthy,
		Selftest: shared.HealthSelftest{
			Version: shared.CurrentVersion,
			Result:  shared.SelftestUnprovable,
			At:      time.Now(),
		},
	})

	if !strings.Contains(body, "the stateful half is matching packets") {
		t.Error("the healthy reason did not reach the page")
	}
	if strings.Contains(body, "The core daemon is running and rules are live") {
		t.Error("the hero still shows the fixed claim although the core answered a health " +
			"state; that sentence is the one this release exists to stop making")
	}
}

// The self-test fact renders nothing where a kernel would go when there is no
// kernel to name.
//
// internal/web has no path to the privileged core.KernelRelease() and imports
// internal/core nowhere, by design, so the demo's unprovable stamp carries the
// zero value while a real host's unprovable stamp carries a release. An empty
// value under a "Kernel" label is a claim about a kernel nobody read; absence is
// not. shared.HealthSelftest.Kernel is omitempty on the wire for the same
// reason.
func TestTheSelftestFactOmitsAnAbsentKernel(t *testing.T) {
	withKernel := dashboardWithHealth(t, shared.HealthResult{
		State:  shared.HealthOK,
		Reason: shared.HealthReasonHealthy,
		Selftest: shared.HealthSelftest{
			Version: shared.CurrentVersion,
			Kernel:  "6.11.0-easywall",
			Result:  shared.SelftestPassed,
			At:      time.Now(),
		},
	})
	if !strings.Contains(withKernel, `class="hero-fact-sub">6.11.0-easywall<`) {
		t.Error("a stamp naming a kernel does not render it")
	}

	without := dashboardWithHealth(t, shared.HealthResult{
		State:  shared.HealthOK,
		Reason: shared.HealthReasonHealthy,
		Selftest: shared.HealthSelftest{
			Version: shared.CurrentVersion,
			Result:  shared.SelftestUnprovable,
			At:      time.Now(),
		},
	})
	if strings.Contains(without, "hero-fact-sub") {
		t.Error("the kernel line is rendered for a stamp that names no kernel; an empty value " +
			"under that label is a claim about a kernel nobody read")
	}
	// The rest of the fact is still there, or this test would pass on a page
	// that dropped the self-test entirely.
	if !strings.Contains(without, "Not provable here") {
		t.Error("the unprovable result itself did not reach the page")
	}
}

// A stamp that has never been written renders no self-test fact at all. A fresh
// install and an unreadable stamp file both look like this, and "Self-test: —"
// is a label about nothing.
func TestTheSelftestFactIsAbsentWhenNothingHasBeenProven(t *testing.T) {
	body := dashboardWithHealth(t, shared.HealthResult{
		State:  shared.HealthOK,
		Reason: shared.HealthReasonHealthy,
	})
	if strings.Contains(body, "Self-test") {
		t.Error("the self-test fact is rendered although no proof has ever been recorded")
	}
	// And the panel around it is: this must not pass because the hero is missing.
	if !strings.Contains(body, "the stateful half is matching packets") {
		t.Fatal("the hero did not render at all, so this assertion proves nothing")
	}
}

// A core too old to answer GET_HEALTH leaves the page exactly as 2.16 rendered
// it, rather than reporting a fault.
//
// The two binaries are packaged separately and a host can run them a version
// apart for the length of an upgrade. Status.Active is Enforcing() — a true
// claim, just a narrower one — so the fallback is the honest answer and not a
// swallowed error.
func TestTheDashboardFallsBackWhenTheCoreCannotAnswerAboutHealth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{Active: true}))
	fc.SetResponse(shared.CmdGetHealth, errorRespFor("unknown command: GET_HEALTH"))

	rec := doAuthRequest(t, s, "GET", "/dashboard", nil)
	if rec.Code != 200 {
		t.Fatalf("GET /dashboard = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "The core daemon is running and rules are live") {
		t.Error("with no health reply the hero must fall back to the enforcing flag, not " +
			"report a fault about a firewall that may be fine")
	}
}
