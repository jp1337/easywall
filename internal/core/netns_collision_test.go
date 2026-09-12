package core

import (
	"net"
	"regexp"
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

// TestWireChecksCollisionBeforeDeletingTheOldLink pins Task 10's actual
// deliverable: the host-collision check has to run, over live interfaces,
// before wire() deletes anything.
//
// The test this replaced, TestARangeCollisionIsUnprovableAndNotAFailure,
// asserted only that errors.Is(err, ErrHarnessRangeInUse) survives
// fmt.Errorf's %w — a property of the standard library, not of this
// package. Proven by mutation, twice: replacing the whole collision block in
// wire() (the net.Interfaces() call, the harnessCollision call, and the
// ErrHarnessRangeInUse return) with `_ = harnessCollision` left both
// collision tests in this file green and the package building. The ordering
// itself had no guard at all.
//
// Reads the source of Harness.wire and compares byte offsets, the same idiom
// TestDaemonStart_SourceRestoresBeforeItListens (daemon_source_order_test.go)
// uses for an order a runtime test cannot reach without a real network
// namespace: net.Interfaces() has to run before harnessCollision reads its
// result, and harnessCollision has to run before deleteLink removes the
// leftover router link — an operator whose LAN already holds the harness
// range must see "unprovable" before that host interface is touched, not
// after.
func TestWireChecksCollisionBeforeDeletingTheOldLink(t *testing.T) {
	body := funcBody(t, coreSource(t, "netns.go"), "netns.go", "func (h *Harness) wire() error {")

	interfaces := regexp.MustCompile(`net\.Interfaces\(\)`)
	collision := regexp.MustCompile(`harnessCollision\(`)
	del := regexp.MustCompile(`deleteLink\(local, harnessRouterIf\)`)

	interfacesAt := interfaces.FindAllStringIndex(body, -1)
	collisionAt := collision.FindAllStringIndex(body, -1)
	delAt := del.FindAllStringIndex(body, -1)

	// Nothing went unparsed. Any of these matching zero times means the call
	// was renamed, moved out of wire, or wrapped in something this test
	// cannot see — in which case the test has stopped guarding anything and
	// has to say so rather than pass on an empty match.
	if len(interfacesAt) != 1 {
		t.Fatalf("want exactly one net.Interfaces() call in Harness.wire, found %d; "+
			"this guard compares against a single call and cannot tell which one "+
			"gathers the interfaces the collision check reads", len(interfacesAt))
	}
	if len(collisionAt) != 1 {
		t.Fatalf("want exactly one harnessCollision( call in Harness.wire, found %d; "+
			"the collision check was renamed, duplicated, or removed, and this guard "+
			"no longer pins its position", len(collisionAt))
	}
	if len(delAt) != 1 {
		t.Fatalf("want exactly one deleteLink(local, harnessRouterIf) call in Harness.wire, "+
			"found %d; the leftover-link cleanup was renamed or moved, and this guard no "+
			"longer pins the order against it", len(delAt))
	}

	if interfacesAt[0][0] > collisionAt[0][0] {
		t.Error("Harness.wire calls harnessCollision before net.Interfaces() gathers what it " +
			"reads; the check would run over stale or zero-value data")
	}
	if collisionAt[0][0] > delAt[0][0] {
		t.Error("Harness.wire deletes the leftover router link before checking for a host " +
			"collision. An operator whose LAN already holds the harness range would have " +
			"that host interface deleted before the self-test ever reports anything, instead " +
			"of failing \"unprovable\" first")
	}
}
