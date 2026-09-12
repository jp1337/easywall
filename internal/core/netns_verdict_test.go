package core

import (
	"errors"
	"net"
	"strings"
	"syscall"
	"testing"
)

// TestPeerVerdictNeverCallsAHarnessFaultAVerdict is the peer-side half of the
// classification inboundCrosses has made since 2.17.
//
// The parent separates ECONNREFUSED from a timeout from anything else,
// precisely so a harness fault is never reported as a verdict. The peer used
// to collapse every dial error into "blocked" — so a harness that broke
// between claim 1's control and its measurement recorded `failed`, which is
// the one inversion foldClaims' doc comment forbids.
//
// It is prevented today by accident and not by design: two concurrent
// RunSelftest() calls collide in wire(), which takes no lock and deletes
// ewst-r unconditionally, and the collision wins the race every time —
// measured as all six claims unprovable, three runs of two.
func TestPeerVerdictNeverCallsAHarnessFaultAVerdict(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"a completed handshake is open", nil, "open"},
		{"a timeout is the only thing a dropping chain produces", timeoutError{}, "blocked"},
		// A refusal is blocked, not a harness fault — the negative control in
		// netns_integration_test.go dials a closed port on the router and
		// requires exactly this answer. inboundCrosses asks the same question
		// ("did a TCP stack answer the SYN") on the other side and gets the
		// same "yes": there it means a packet crossed into an empty
		// namespace; here it means the harness's own bound listener declined
		// it. Either way a stack answered, so it is a verdict, not a fault.
		{"a refusal against a bound listener is blocked, not a harness fault", syscall.ECONNREFUSED, "blocked"},
		{"an unreachable network is the harness", syscall.ENETUNREACH, "failed"},
		{"an unreachable host is the harness", syscall.EHOSTUNREACH, "failed"},
		{"anything nobody thought of is the harness", errors.New("something else"), "failed"},
		// The only case that can falsify the one-line assertion below. The four
		// above all return single-line Error() strings, so deleting peerVerdict's
		// strings.Fields/Join flattening would leave them all green.
		{"a multi-line error is flattened to one line", errors.New("dial failed:\nconnection reset"), "failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := peerVerdict(tc.err)
			if tc.want == "failed" {
				if !strings.HasPrefix(got, "failed ") {
					t.Fatalf("peerVerdict(%v) = %q — a harness fault must not be a verdict", tc.err, got)
				}
				if strings.ContainsAny(got, "\n\r") {
					t.Errorf("the reason spans lines: %q — the pipe protocol is one line each way", got)
				}
				return
			}
			if got != tc.want {
				t.Errorf("peerVerdict(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// TestTheParentRefusesToReadAFailureAsAVerdict is the other end of the wire.
func TestTheParentRefusesToReadAFailureAsAVerdict(t *testing.T) {
	crossed, err := interpretPeerLine("failed connect: network is unreachable")
	if err == nil {
		t.Fatal("a harness failure was read as a verdict")
	}
	if crossed {
		t.Error("a harness failure reported a packet crossing")
	}
	if !strings.Contains(err.Error(), "network is unreachable") {
		t.Errorf("the error drops the peer's reason, which is the whole point: %v", err)
	}

	if crossed, err := interpretPeerLine("open"); err != nil || !crossed {
		t.Errorf(`interpretPeerLine("open") = %v, %v — want true, nil`, crossed, err)
	}
	if crossed, err := interpretPeerLine("blocked"); err != nil || crossed {
		t.Errorf(`interpretPeerLine("blocked") = %v, %v — want false, nil`, crossed, err)
	}
	if _, err := interpretPeerLine("unparsed"); err == nil {
		t.Error(`"unparsed" was read as a verdict`)
	}
}

// timeoutError is a net.Error that reports a timeout, which is what a dial
// against a dropping chain produces.
type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

var _ net.Error = timeoutError{}
