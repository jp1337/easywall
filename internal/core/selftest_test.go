package core

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// The property that replaces a landmine: the order of the claims cannot change
// the result.
//
// The provers are not independent — claims 3 and 4 use the open port as their
// control, so a genuinely broken port rule leaves those two unable to settle
// while only claim 2 reports it as false. The first version of RunSelftest
// returned at the first claim it could not settle, so listing claim 3 first
// turned a broken firewall into "this host cannot be asked". A review found it
// by swapping two lines.
//
// This asserts the fix rather than the symptom. Every permutation of one false
// claim among two unsettleable ones must read `failed`, and the detail must
// name the claim that was actually false — never one of the two that could not
// be asked. A guard that checked only the shipped order would go green again
// the moment somebody reordered the list, which is precisely the edit that
// produced the defect.
func TestSelftestFailedOutranksUnprovableInEveryOrder(t *testing.T) {
	// What a broken port-accept verdict does to the four provers: claim 2
	// measures it and says so, claims 3 and 4 cannot get their control refused
	// and so cannot settle at all, claim 1 is untouched.
	passes := selftestClaim{"claim that passes", func() (bool, string, error) {
		return true, "", nil
	}}
	isFalse := selftestClaim{"the claim that is actually false", func() (bool, string, error) {
		return false, "the rule that opens it accepted nothing", nil
	}}
	cannotSettleA := selftestClaim{"a claim whose control was refused", func() (bool, string, error) {
		return false, "", errors.New("control refused: open port was not answered")
	}}
	cannotSettleB := selftestClaim{"another claim whose control was refused", func() (bool, string, error) {
		return false, "", errors.New("control refused: open port was not answered")
	}}

	orders := [][]selftestClaim{
		{passes, isFalse, cannotSettleA, cannotSettleB}, // the shipped order
		{passes, cannotSettleA, isFalse, cannotSettleB}, // the review's swap
		{cannotSettleA, cannotSettleB, passes, isFalse}, // the false claim last
		{isFalse, passes, cannotSettleA, cannotSettleB}, // and first
	}
	for i, order := range orders {
		result, detail := foldClaims(order)
		if result != shared.SelftestFailed {
			t.Errorf("order %d: result = %q, want %q; a claim that is false must outrank every "+
				"claim that could not be settled, or a broken firewall reads as an unaskable host",
				i, result, shared.SelftestFailed)
		}
		if !strings.HasPrefix(detail, isFalse.name) {
			t.Errorf("order %d: detail = %q; it must name the claim that was false, not one of "+
				"the claims that could not be asked", i, detail)
		}
	}
}

// The other half of the same rule, and the one that must not be lost while
// fixing the first: an error is never a red firewall.
//
// `unprovable` is the ordinary answer in production — easywall-core.service
// bounds the capability set to CAP_NET_ADMIN and CLONE_NEWNET needs
// CAP_SYS_ADMIN — so folding a claim nobody could ask into `failed` would put
// a red state on every container and every hardened host.
func TestSelftestUnprovableOnlyWhenNothingSettled(t *testing.T) {
	cannotSettle := func(name string) selftestClaim {
		return selftestClaim{name, func() (bool, string, error) {
			return false, "", errors.New("a network namespace cannot be created here")
		}}
	}
	result, detail := foldClaims([]selftestClaim{cannotSettle("first"), cannotSettle("second")})
	if result != shared.SelftestUnprovable {
		t.Errorf("result = %q, want %q", result, shared.SelftestUnprovable)
	}
	if !strings.HasPrefix(detail, "first") {
		t.Errorf("detail = %q; it must name the first claim that could not be settled", detail)
	}

	result, detail = foldClaims([]selftestClaim{
		{"a", func() (bool, string, error) { return true, "", nil }},
		{"b", func() (bool, string, error) { return true, "", nil }},
	})
	if result != shared.SelftestPassed {
		t.Errorf("result = %q, want %q", result, shared.SelftestPassed)
	}
	// A passed stamp with a detail would be a proof that both succeeded and had
	// a complaint, which is the shape of a claim that was not really made.
	if detail != "" {
		t.Errorf("detail = %q on a passed fold; a proof that passed has nothing to say", detail)
	}
}

// RunSelftest end to end on a host that cannot build a namespace — which is the
// usual host. startPeer is a var for exactly this, and netns_test.go already
// substitutes it, so this runs under `make test` with no kernel, no capability
// and no root.
//
// The three metadata fields are the point as much as the result. Health reads
// the stamp and StampStore.Stale compares Version *and* Kernel, so a stamp with
// a result and a blank version would make every daemon start stale and re-run a
// proof that already answered. They must be filled on the unprovable path too,
// which is the path nothing else covers.
func TestSelftestStampAlwaysCarriesVersionKernelAndTime(t *testing.T) {
	original := startPeer
	t.Cleanup(func() { startPeer = original })
	calls := 0
	startPeer = func() (*exec.Cmd, *os.File, *os.File, error) {
		// What clone(CLONE_NEWNET) returns without CAP_SYS_ADMIN.
		calls++
		return nil, nil, nil, syscall.EPERM
	}

	before := time.Now().UTC()
	stamp := RunSelftest()

	if stamp.Result != shared.SelftestUnprovable {
		t.Errorf("Result = %q, want %q; a refused clone is not a finding about the firewall",
			stamp.Result, shared.SelftestUnprovable)
	}
	if !strings.Contains(stamp.Detail, ErrNamespaceUnavailable.Error()) {
		t.Errorf("Detail = %q; it must carry the reason the proof could not run", stamp.Detail)
	}
	if stamp.Version != shared.CurrentVersion {
		t.Errorf("Version = %q, want %q", stamp.Version, shared.CurrentVersion)
	}
	if stamp.Kernel != KernelRelease() {
		t.Errorf("Kernel = %q, want %q", stamp.Kernel, KernelRelease())
	}
	if stamp.At.Before(before) || stamp.At.After(time.Now().UTC().Add(time.Second)) {
		t.Errorf("At = %v, which is not the moment the proof ran", stamp.At)
	}
	// Every claim is attempted, not only the first. That is what makes the
	// order of the list stop mattering, and a fold that returned early would
	// show up here as a single call.
	if calls != len(selftestClaims()) {
		t.Errorf("startPeer was called %d times, want %d; every claim has to be attempted or the "+
			"order of the list decides the outcome again", calls, len(selftestClaims()))
	}
}

// The claim list is what RunSelftest folds, and a claim that is not in it is a
// proof nobody ever runs. Asserted by description because that is what an
// operator reads in the stamp, and because a prover reachable only from a test
// is exactly the shape this release exists to remove: a surface that reports a
// verdict nothing ever took.
func TestRunSelftest_ClaimsTheForwardedPortCase(t *testing.T) {
	want := "a forwarded rule opens one container port and the deny closes the rest"
	for _, c := range selftestClaims() {
		if c.name == want {
			if c.prove == nil {
				t.Fatalf("the forwarded-port claim is in the list with no prover behind it")
			}
			return
		}
	}
	t.Fatalf("the forwarded-port claim is missing from the selftest")
}
