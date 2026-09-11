package core

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
)

// fakeAddr is a net.Addr carrying a CIDR string, which is what
// net.Interface.Addrs returns in practice (*net.IPNet).
func cidr(t *testing.T, s string) net.Addr {
	t.Helper()
	_, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("ParseCIDR(%q): %v", s, err)
	}
	return ipnet
}

// TestHarnessCollisionNamesWhatItFound covers the case nothing checked.
//
// The harness range and both interface names are fixed constants, and wire()
// deletes any host ewst-r unconditionally — which is right, and documented:
// a per-pid name would collide one step later on 10.77.9.1/24 because
// addAddress is Create|Excl. What was missing is the host that already uses
// the range. An operator whose LAN is 10.77.9.0/24 got a harness failure
// instead of "unprovable", on a release whose subject is not lying about the
// firewall's state.
//
// ewst-r is deliberately NOT a collision here: its deletion is what makes a
// second NewHarness in the same process work at all, since the previous
// router end outlives its namespace by about 110 ms.
func TestHarnessCollisionNamesWhatItFound(t *testing.T) {
	tests := []struct {
		name     string
		ifaces   []net.Interface
		addrs    map[string][]net.Addr
		wantHit  bool
		wantText string
	}{
		{
			name:   "an ordinary host is no collision",
			ifaces: []net.Interface{{Name: "lo"}, {Name: "eth0"}},
			addrs: map[string][]net.Addr{
				"lo":   {cidr(t, "127.0.0.1/8")},
				"eth0": {cidr(t, "192.168.1.10/24")},
			},
			wantHit: false,
		},
		{
			name:   "a LAN on the harness range collides",
			ifaces: []net.Interface{{Name: "eth0"}},
			addrs: map[string][]net.Addr{
				"eth0": {cidr(t, "10.77.9.40/24")},
			},
			wantHit:  true,
			wantText: "eth0",
		},
		{
			name:   "the harness's own router end is not a collision",
			ifaces: []net.Interface{{Name: harnessRouterIf}},
			addrs: map[string][]net.Addr{
				harnessRouterIf: {cidr(t, "10.77.9.1/24")},
			},
			wantHit: false,
		},
		{
			name:     "a host interface called ewst-p collides",
			ifaces:   []net.Interface{{Name: harnessPeerIf}},
			addrs:    map[string][]net.Addr{harnessPeerIf: nil},
			wantHit:  true,
			wantText: harnessPeerIf,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := harnessCollision(tc.ifaces, func(i net.Interface) ([]net.Addr, error) {
				return tc.addrs[i.Name], nil
			})
			if err != nil {
				t.Fatalf("harnessCollision: %v", err)
			}
			if tc.wantHit && got == "" {
				t.Fatal("a collision was not reported; the self-test would fail rather than report unprovable")
			}
			if !tc.wantHit && got != "" {
				t.Fatalf("a collision was reported where there is none: %q", got)
			}
			if tc.wantHit && !strings.Contains(got, tc.wantText) {
				t.Errorf("the detail does not name what collided: %q, want it to mention %q", got, tc.wantText)
			}
		})
	}
}

// TestARangeCollisionIsUnprovableAndNotAFailure pins the classification.
//
// A sentinel nothing checks is a comment. This asserts the caller treats a
// collision the way it treats an absent namespace: unprovable with a detail,
// never a verdict about the firewall.
//
// foldClaims (selftest.go) does not switch on ErrNamespaceUnavailable
// specifically — every error from a prover, wrapped or not, becomes
// "unprovable" via a bare `err != nil` check. So ErrHarnessRangeInUse needs no
// classification of its own; this test just pins that it survives wrapping,
// which is what errors.Is(err, ErrNamespaceUnavailable) relies on elsewhere in
// this file's own tests.
func TestARangeCollisionIsUnprovableAndNotAFailure(t *testing.T) {
	err := fmt.Errorf("%w: eth0 holds 10.77.9.40, inside the harness range 10.77.9.0/24", ErrHarnessRangeInUse)
	if !errors.Is(err, ErrHarnessRangeInUse) {
		t.Fatal("the sentinel does not survive wrapping")
	}
}
