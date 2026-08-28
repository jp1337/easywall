package web

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// serverWithTrusted builds a Server whose configuration trusts one address.
// Nothing else about it is exercised here.
func serverWithTrusted(t *testing.T, trusted ...string) *Server {
	t.Helper()
	cfg := &Config{}
	cfg.WebConfig = shared.WebDefault()
	cfg.TrustedProxies = trusted
	return &Server{cfg: cfg}
}

// The address the audit log records is the resolved client, and the via-proxy
// marker fires whenever the walk could not name one — on an untrusted peer's
// header, and on a trusted peer with no usable header of its own. Spec §3,
// rows one and two.
//
// Routed through recordLoginEvent itself, using the same fakeCore-backed
// Server and payload-capture shape as auditevents_test.go's
// TestAuditEvents_TheEventReachesTheCore: asserting on what clientAddr
// returns proves only the accessor. A regression inside recordLoginEvent —
// reading peerIP/proxiedRequest directly again, which is 2.12 restored —
// compiles cleanly and would still pass a test that stops at clientAddr.
func TestTheRecordedAddressIsTheResolvedClient(t *testing.T) {
	for _, tc := range []struct {
		name        string
		trusted     []string
		peer        string
		header      string
		wantAddr    string
		wantProxied bool
	}{
		{"an untrusted peer, no header", nil, "203.0.113.7:5555", "", "203.0.113.7", false},
		{"an untrusted peer, a header", nil, "203.0.113.7:5555", "198.51.100.1", "203.0.113.7", true},
		{"a trusted peer, a usable header", []string{"10.1.0.5"}, "10.1.0.5:41000", "198.51.100.1", "198.51.100.1", false},
		{"a trusted peer, no usable header", []string{"10.1.0.5"}, "10.1.0.5:41000", "", "10.1.0.5", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fc := newFakeCore(t)
			seen := make(chan shared.Command, 1)
			fc.OnCommand(shared.CmdLogEvent, func(c shared.Command) { seen <- c })

			s := newTestServer(t, fc)
			s.cfg.TrustedProxies = tc.trusted

			r := httptest.NewRequest("POST", "/login", nil)
			r.RemoteAddr = tc.peer
			if tc.header != "" {
				r.Header.Set("X-Forwarded-For", tc.header)
			}

			s.recordLoginEvent(r, shared.EvLoginOK, 0)

			select {
			case cmd := <-seen:
				var p shared.LogEventPayload
				if err := json.Unmarshal(cmd.Payload, &p); err != nil {
					t.Fatalf("decode CmdLogEvent payload: %v", err)
				}
				if p.Addr != tc.wantAddr {
					t.Errorf("address = %q, want %q", p.Addr, tc.wantAddr)
				}
				if p.Proxied != tc.wantProxied {
					t.Errorf("proxied = %v, want %v", p.Proxied, tc.wantProxied)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("the event never reached the core")
			}
		})
	}
}

// A trusted peer gets a real reachability verdict instead of "cannot tell".
// Spec §3, row three: today any header at all moves the verdict to unknown,
// which behind a proxy is every request forever.
//
// Goes through reachVerdict itself, not a hand-rolled call to
// shared.Reachable: the wiring inside reachVerdict — that it asks clientAddr
// rather than the raw peer, and passes the result on unchanged — is the
// entire substance of this task, and a test that reconstructs the function's
// body instead of calling it proves nothing about that wiring (spec §4.4).
func TestATrustedPeerGetsARealVerdict(t *testing.T) {
	s := serverWithTrusted(t, "10.1.0.5")
	r := httptest.NewRequest("GET", "/apply", nil)
	r.RemoteAddr = "10.1.0.5:41000"
	r.Header.Set("X-Forwarded-For", "198.51.100.1")

	v := s.reachVerdict(r, shared.Rules{}, shared.FirewallOptions{}, shared.NetworkSettings{})
	if v.Verdict == shared.ReachUnknown && v.Reason == shared.ReasonProxied {
		t.Error("the verdict is still 'cannot tell' for a trusted peer")
	}
	if v.Addr != "198.51.100.1" {
		t.Errorf("verdict address = %q, want the resolved client %q", v.Addr, "198.51.100.1")
	}
}

// A trusted proxy that forwards a loopback address makes the resolved client
// look local to reachVerdict's locality test. That is inside the documented
// trust boundary — being on the list is total trust in that peer — and not a
// defect: reasoning about the *proxy's* locality instead is exactly what this
// release exists to stop. Pinned here so the next reader finds a decision,
// not a surprise.
//
// Through reachVerdict itself, for the same reason as the test above: the
// point being pinned is that the *resolved* address reaches shared.Reachable,
// not the peer, and only a call to reachVerdict exercises that routing.
func TestATrustedProxyCanMakeARemoteCallerLookLocal(t *testing.T) {
	s := serverWithTrusted(t, "10.1.0.5")
	r := httptest.NewRequest("GET", "/apply", nil)
	r.RemoteAddr = "10.1.0.5:41000"
	r.Header.Set("X-Forwarded-For", "127.0.0.1")

	v := s.reachVerdict(r, shared.Rules{}, shared.FirewallOptions{}, shared.NetworkSettings{})
	if v.Addr != "127.0.0.1" {
		t.Fatalf("verdict address = %q, want the resolved (loopback) client %q", v.Addr, "127.0.0.1")
	}
	if v.Verdict != shared.ReachOpen || v.Reason != shared.ReasonLoopback {
		t.Errorf("verdict = %s/%s, want %s/%s — a trusted proxy's forwarded loopback address "+
			"made the resolved client look local, which is inside the trust boundary, not a defect",
			v.Verdict, v.Reason, shared.ReachOpen, shared.ReasonLoopback)
	}
}
