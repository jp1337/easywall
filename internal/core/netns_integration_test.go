//go:build integration

package core

import (
	"errors"
	"net"
	"testing"
	"time"
)

// The harness against a real kernel. If this passes, the self-test has a floor:
// a packet crosses a veth into this namespace and is decided by the input
// chain, which is the one thing a probe against the host's own address can
// never show — local delivery goes out lo and the chain accepts "iif lo" before
// it reaches anything else.
//
// Nothing here writes a rule. This measures the harness, not the firewall: an
// unreachable peer and a firewall that is dropping look identical from the
// outside, so the open case has to be established before any rule exists.
func TestIntegration_HarnessCarriesAPacket(t *testing.T) {
	h, err := NewHarness()
	if errors.Is(err, ErrNamespaceUnavailable) {
		skipOrFailUnprovable(t, err.Error())
	}
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	if h.NetNSFd() < 0 {
		t.Error("NetNSFd is negative; the self-test hands this to nftables.WithNetNSFd " +
			"and would write its table into this namespace instead of the peer's")
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(h.RouterAddr().String(), "12227"))
	if err != nil {
		t.Fatalf("listen on the router side: %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	// No easywall table in this namespace yet, so the default policy decides
	// and the connection must succeed.
	open, err := h.Dial(h.RouterAddr(), 12227, 2*time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if !open {
		t.Fatal("the peer could not reach the router with no firewall in the way; " +
			"the harness is not routing")
	}

	// And the negative, so that a later "blocked" verdict means something: a
	// port nobody is listening on has to come back blocked. A harness whose
	// Dial answered "open" unconditionally would pass the assertion above and
	// prove nothing at all about a closed port.
	open, err = h.Dial(h.RouterAddr(), 12226, 2*time.Second)
	if err != nil {
		t.Fatalf("Dial to a closed port: %v", err)
	}
	if open {
		t.Error("the peer reported a port nobody is listening on as open")
	}
}

// Three harnesses back to back, each built immediately after the previous one
// was closed, and each one made to carry a packet.
//
// Nothing in the suite did this until a review found why it matters, which is
// how a Critical survived a whole round: Close returns long before the kernel's
// cleanup_net destroys the peer's namespace, so the previous router-side end is
// still registered in this namespace — about 110 ms on an idle container, and
// longer under load, because netns teardown is batched. The second NewHarness
// therefore met EEXIST, wrapped as "creating the veth pair", which is not
// ErrNamespaceUnavailable: the self-test would have called the harness broken
// rather than the proof unavailable.
//
// No sleep anywhere here on purpose. A wait would hide the defect this test
// exists for, and would only hide it on a machine as idle as the one it was
// written on.
func TestIntegration_HarnessCanBeBuiltTwice(t *testing.T) {
	for i := 1; i <= 3; i++ {
		h, err := NewHarness()
		if errors.Is(err, ErrNamespaceUnavailable) {
			skipOrFailUnprovable(t, err.Error())
		}
		if err != nil {
			t.Fatalf("harness %d of 3: %v", i, err)
		}

		// Built is not enough: the leftover has to be gone in a way that leaves
		// a working pair behind, not merely a successful RTM_NEWLINK.
		ln, err := net.Listen("tcp", net.JoinHostPort(h.RouterAddr().String(), "12225"))
		if err != nil {
			h.Close()
			t.Fatalf("harness %d of 3, listen on the router side: %v", i, err)
		}
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				_ = conn.Close()
			}
		}()

		open, err := h.Dial(h.RouterAddr(), 12225, 2*time.Second)
		_ = ln.Close()
		h.Close()
		if err != nil {
			t.Fatalf("harness %d of 3, Dial: %v", i, err)
		}
		if !open {
			t.Fatalf("harness %d of 3 was built but does not route", i)
		}
	}
}
