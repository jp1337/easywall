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
		t.Skipf("skipping: %v", err)
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
