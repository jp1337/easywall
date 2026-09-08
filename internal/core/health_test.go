package core

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The state machine of spec §2, as a table. Order matters: not-enforcing beats
// everything, panic beats the counter, and the stamp is read last.
func TestHealthStateMachine(t *testing.T) {
	cases := []struct {
		name        string
		enforcing   bool
		panicOn     bool
		established uint64
		ports       uint64
		stamp       shared.SelftestResult
		findings    int
		wantState   shared.HealthState
		wantReason  shared.HealthReason
	}{
		{name: "no table", enforcing: false,
			wantState: shared.HealthFail, wantReason: shared.HealthReasonNotEnforcing},
		{name: "no table beats a failed proof and a finding", enforcing: false,
			panicOn: true, established: 0, ports: 40, stamp: shared.SelftestFailed, findings: 2,
			wantState: shared.HealthFail, wantReason: shared.HealthReasonNotEnforcing},
		{name: "panic beats a healthy table", enforcing: true, panicOn: true,
			established: 10, ports: 10, stamp: shared.SelftestPassed,
			wantState: shared.HealthDegraded, wantReason: shared.HealthReasonPanic},
		{name: "panic beats the counter", enforcing: true, panicOn: true,
			established: 0, ports: 40, stamp: shared.SelftestPassed,
			wantState: shared.HealthDegraded, wantReason: shared.HealthReasonPanic},
		// The case the mutation "check the stamp before panic" needs, and the
		// reason it is here rather than left to the row above: a *passed* stamp
		// never trips a stamp check wherever it sits, so moving that check ahead
		// of panic is invisible to every case whose stamp passed. Only a panic
		// engaged on a host whose proof also failed tells the two orders apart.
		{name: "panic beats a failed proof and a finding", enforcing: true, panicOn: true,
			established: 5, ports: 5, stamp: shared.SelftestFailed, findings: 2,
			wantState: shared.HealthDegraded, wantReason: shared.HealthReasonPanic},
		{name: "the stateful half matches nothing", enforcing: true,
			established: 0, ports: 40, stamp: shared.SelftestPassed,
			wantState: shared.HealthDegraded, wantReason: shared.HealthReasonStatefulDead},
		{name: "the counter beats a failed proof", enforcing: true,
			established: 0, ports: 40, stamp: shared.SelftestFailed,
			wantState: shared.HealthDegraded, wantReason: shared.HealthReasonStatefulDead},
		{name: "a silent host is healthy", enforcing: true,
			established: 0, ports: 0, stamp: shared.SelftestPassed,
			wantState: shared.HealthOK, wantReason: shared.HealthReasonHealthy},
		{name: "outbound-only traffic is healthy", enforcing: true,
			established: 900, ports: 0, stamp: shared.SelftestPassed,
			wantState: shared.HealthOK, wantReason: shared.HealthReasonHealthy},
		{name: "a failed proof degrades", enforcing: true,
			established: 5, ports: 5, stamp: shared.SelftestFailed,
			wantState: shared.HealthDegraded, wantReason: shared.HealthReasonSelftestFailed},
		{name: "unprovable is not degraded", enforcing: true,
			established: 5, ports: 5, stamp: shared.SelftestUnprovable,
			wantState: shared.HealthOK, wantReason: shared.HealthReasonHealthy},
		{name: "no stamp at all is not degraded", enforcing: true,
			established: 5, ports: 5, stamp: "",
			wantState: shared.HealthOK, wantReason: shared.HealthReasonHealthy},
		{name: "a build finding degrades", enforcing: true,
			established: 5, ports: 5, stamp: shared.SelftestPassed, findings: 1,
			wantState: shared.HealthDegraded, wantReason: shared.HealthReasonBuildFindings},
		{name: "a build finding beats a failed proof", enforcing: true,
			established: 5, ports: 5, stamp: shared.SelftestFailed, findings: 1,
			wantState: shared.HealthDegraded, wantReason: shared.HealthReasonBuildFindings},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeHealth(healthInputs{
				enforcing:       tc.enforcing,
				panicEngaged:    tc.panicOn,
				establishedPkts: tc.established,
				portPkts:        tc.ports,
				stamp:           shared.SelftestStamp{Result: tc.stamp},
				buildFindings:   tc.findings,
			})
			if got.State != tc.wantState || got.Reason != tc.wantReason {
				t.Errorf("= %s/%s, want %s/%s",
					got.State, got.Reason, tc.wantState, tc.wantReason)
			}
		})
	}
}

// Named separately because it is the one false positive that would get the
// alarm switched off: a host with no traffic at all.
func TestHealthIsOkOnASilentHost(t *testing.T) {
	got := computeHealth(healthInputs{enforcing: true,
		establishedPkts: 0, portPkts: 0, stamp: shared.SelftestStamp{Result: shared.SelftestPassed}})
	if got.State != shared.HealthOK {
		t.Errorf("a host with no traffic reads %s; established == 0 is honest there",
			got.State)
	}
}

// The reply reaches an unauthenticated endpoint. It must carry no rule detail.
//
// The forbidden substrings are checked against the marshalled reply and not
// against a field, deliberately: a future field carrying Detail under another
// name — or an embedded SelftestStamp — is what this has to catch, and neither
// would be visible to an assertion that named HealthSelftest's fields.
func TestHealthResultCarriesNoRuleDetail(t *testing.T) {
	got := computeHealth(healthInputs{enforcing: true, buildFindings: 3,
		stamp: shared.SelftestStamp{Result: shared.SelftestFailed,
			Detail: "an open port accepts a connection — port 12227 was dropped"}})
	blob, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"12227", "port"} {
		if strings.Contains(string(blob), leak) {
			t.Errorf("the health reply contains %q: %s", leak, blob)
		}
	}
}

// Health() itself, over the one Firewall a unit test can build: no netlink
// connection, no stamp store, a data directory with nothing in it.
//
// The gathering half has no other cover — computeHealth is where the branches
// are and this is everything around it — and there is one specific way for it
// to fail. The helpers in daemon_test.go, restore_test.go and
// firewall_integration_test.go all construct Firewall literals with the three
// or four fields they need, so f.stamp is nil in every one of them; a later
// task wiring GET_HEALTH into any of those would have found a nil-pointer
// panic in the privileged process rather than a test failure.
func TestHealthSurvivesAFirewallBuiltWithoutAStampStore(t *testing.T) {
	f := &Firewall{
		cfg: &Config{CoreConfig: shared.CoreConfig{DataDir: t.TempDir()}},
		nft: &NftablesManager{},
	}

	got := f.Health()

	// No netlink connection means Enforcing reads false, which is the honest
	// answer to "I cannot confirm the firewall is up" and the first branch of
	// the machine.
	if got.State != shared.HealthFail || got.Reason != shared.HealthReasonNotEnforcing {
		t.Errorf("= %s/%s, want %s/%s", got.State, got.Reason,
			shared.HealthFail, shared.HealthReasonNotEnforcing)
	}
	if got.Selftest.Result != "" {
		t.Errorf("the stamp reads %q on a Firewall that has no stamp store",
			got.Selftest.Result)
	}
}

// Every reason the state machine can return has to be in AllHealthReasons,
// because that slice is what the locale guard iterates: a reason added to
// computeHealth and not to the list reaches a page with no translation and
// nothing says so. The reverse — a listed reason no code produces — is a dead
// locale key and is checked here too.
func TestEveryHealthReasonIsListed(t *testing.T) {
	listed := map[shared.HealthReason]bool{}
	for _, r := range shared.AllHealthReasons {
		listed[r] = true
	}

	produced := map[shared.HealthReason]bool{}
	for _, in := range []healthInputs{
		{enforcing: false},
		{enforcing: true, panicEngaged: true},
		{enforcing: true, portPkts: 1},
		{enforcing: true, buildFindings: 1},
		{enforcing: true, stamp: shared.SelftestStamp{Result: shared.SelftestFailed}},
		{enforcing: true},
	} {
		produced[computeHealth(in).Reason] = true
	}

	for r := range produced {
		if !listed[r] {
			t.Errorf("computeHealth returns %q, which AllHealthReasons does not list", r)
		}
	}
	for _, r := range shared.AllHealthReasons {
		if !produced[r] {
			t.Errorf("AllHealthReasons lists %q, which no input above produces", r)
		}
	}
}
