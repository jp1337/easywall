//go:build integration

package core

import (
	"os"
	"testing"
)

// requireSelftestEnv is set in CI, and its whole job is to make a skip fail.
//
// Three SKIPs and three PASSes are the same green tick. `go test` prints the
// skip reason and moves on, the job exits 0, and nothing distinguishes "the four
// claims were proven against a real kernel" from "nothing was measured". Worse
// than a skip, measured: with every prover skipping,
// TestIntegration_SelftestProvesTheRemainingThreeClaims reports `--- PASS`,
// because a parent test whose subtests all skip is not itself skipped.
//
// **What this actually covers, and what it does not.** The obvious hazard — the
// whole suite losing CAP_SYS_ADMIN to a runner image or a sandbox change — is
// already loud: TestMain re-execs into CLONE_NEWNET and os.Exit(1)s when it
// cannot, so the job fails with no test output at all. Naming that as the reason
// would be naming a hazard something else already handles, which is how a guard
// gets deleted by the next reader who checks.
//
// The case this catches is the narrower and quieter one: TestMain's clone
// succeeded, the suite is running, and a prover nevertheless returns
// "unprovable" — the harness's veth pair or its second namespace failing, a
// netlink call refused, a partial capability set that is enough for one clone
// and not for the rest. That is a green run measuring nothing, behind a suite
// that started perfectly well. It is exactly what the mutation making NewHarness
// return ErrNamespaceUnavailable unconditionally reproduces, and it is the shape
// of a CI gate running green with an empty accumulator, which this release has
// already produced twice.
//
// Deliberately in the test suite and not in a grep over the run's output — an
// output grep drifts the moment a test is renamed, and it drifts silently in the
// passing direction.
//
// The name is also spelled out in .github/workflows/test.yml, and
// TestTheCIProofGateIsStillWiredUp (internal/shared) is what keeps the two
// spellings the same: renaming it here and not there would disable the gate
// without a single test going red.
const requireSelftestEnv = "EASYWALL_REQUIRE_SELFTEST"

// skipOrFailUnprovable is the one place a self-test prover is allowed to give up
// on a host that cannot build the harness. On a developer's machine it skips,
// which is right — rootless `unshare -n` is refused on plenty of them, and the
// three-state result exists precisely so that "cannot prove" does not read as
// "broken firewall". In CI it fails.
func skipOrFailUnprovable(t *testing.T, reason string) {
	t.Helper()
	if os.Getenv(requireSelftestEnv) != "" {
		t.Fatalf("nothing was proven here, and %s is set: %s\n"+
			"  This job exists to run the proof, so a skip is a failure. If a runner "+
			"image or a sandbox change has taken CAP_SYS_ADMIN away, fix that — do "+
			"not unset the variable, or the tick goes back to being green for a suite "+
			"that measures nothing.", requireSelftestEnv, reason)
	}
	t.Skipf("skipping: %s", reason)
}
