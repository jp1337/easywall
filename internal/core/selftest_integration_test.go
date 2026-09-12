//go:build integration

package core

import (
	"errors"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// Layer C. Five claims about the table easywall builds, measured with a real
// packet in a namespace of C's own. The host's real table is never touched.
//
// The skip is not a formality. easywall-core.service grants CAP_NET_ADMIN and
// bounds the set to it, and CLONE_NEWNET needs CAP_SYS_ADMIN, so "unprovable"
// is the ordinary answer on most hosts and in most containers — and the whole
// point of the three-state result is that it must not read as a broken
// firewall. Run this suite without --cap-add=SYS_ADMIN and read the skip
// message: a green run that silently proved nothing is the failure mode this
// release exists to remove.
func TestIntegration_SelftestProvesTheFiveClaims(t *testing.T) {
	stamp := RunSelftest()
	if stamp.Result == shared.SelftestUnprovable {
		skipOrFailUnprovable(t, stamp.Detail)
	}
	if stamp.Result != shared.SelftestPassed {
		t.Fatalf("RunSelftest = %s: %s", stamp.Result, stamp.Detail)
	}
	if stamp.Kernel != KernelRelease() {
		t.Errorf("Kernel = %q, want %q", stamp.Kernel, KernelRelease())
	}
	if stamp.Version != shared.CurrentVersion {
		t.Errorf("Version = %q, want %q", stamp.Version, shared.CurrentVersion)
	}
	if stamp.At.IsZero() {
		t.Error("At is the zero time")
	}
	// A passed stamp with a detail would mean a prover reported both an outcome
	// and a complaint, which is the shape of a claim that was not really made.
	if stamp.Detail != "" {
		t.Errorf("Detail = %q on a passed stamp; a proof that passed has nothing to say", stamp.Detail)
	}
}

// The regression test for the reported defect, measured rather than decoded.
// Layer B catches the reversed mask by reading the expression; this catches it
// by finding that a reply gets no answer, which is what the operator saw.
//
// The claim in isolation, so a failure names which one broke — and so that the
// mutation that matters (ctStateMask back on BigEndian) has one test to turn
// red rather than a stamp string to be read.
func TestIntegration_SelftestProvesEstablishedPasses(t *testing.T) {
	ok, detail, err := proveEstablishedPasses()
	if errors.Is(err, ErrNamespaceUnavailable) {
		skipOrFailUnprovable(t, err.Error())
	}
	if err != nil {
		t.Fatalf("this claim could not be settled here, so nothing was proven: %v", err)
	}
	if !ok {
		t.Fatalf("a reply on an established connection did not pass: %s", detail)
	}
}

// Each remaining claim on its own, for the same reason: the mutation table in
// the brief names a prover per mutation, and a stamp that stops at the first
// false claim would report only the first of them.
func TestIntegration_SelftestProvesTheRemainingFourClaims(t *testing.T) {
	for _, c := range []struct {
		name  string
		prove func() (bool, string, error)
	}{
		{"an open port accepts a connection", proveOpenPortAccepts},
		{"a closed port does not", proveClosedPortRefuses},
		{"a blacklisted address does not reach an open port", proveBlacklistWins},
		{"a forwarded rule opens one container port and the deny closes the rest",
			proveForwardedPortFiltered},
	} {
		t.Run(c.name, func(t *testing.T) {
			ok, detail, err := c.prove()
			if errors.Is(err, ErrNamespaceUnavailable) {
				skipOrFailUnprovable(t, err.Error())
			}
			if err != nil {
				t.Fatalf("this claim could not be settled here, so nothing was proven: %v", err)
			}
			if !ok {
				t.Fatalf("%s: %s", c.name, detail)
			}
		})
	}
}
