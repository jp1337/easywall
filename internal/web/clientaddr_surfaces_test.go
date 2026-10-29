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

// Behind a trusted proxy the verdict is "cannot tell", and says why. 2.13 gave
// it a real verdict for the client the proxy named, computed for easywall's own
// listen port — but the client's SYN goes to the proxy's port (443 in
// reverse-proxy.md), and a remote proxy's clients never reach this host at all.
// Measured with webPort 19999: only 443 open said blocked/no_rule, only 19999
// open said open/port_open to an operator the apply was cutting off at 443.
//
// Through reachVerdict itself, not shared.Reachable: the wiring is the
// substance. The loopback case is the one 2.13 pinned as "open/loopback": a
// forwarded 127.0.0.1 is no more a port than any other client.
func TestATrustedPeerGetsCannotTellNotTheWrongPort(t *testing.T) {
	for _, tc := range []struct {
		name, proxy, client, open string
	}{
		{"only the proxy's port open", "127.0.0.1", "203.0.113.5", "443"},
		{"only easywall's own port open", "127.0.0.1", "203.0.113.5", "19999"},
		{"a remote proxy forwarding loopback", "10.1.0.5", "127.0.0.1", "19999"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := serverWithTrusted(t, tc.proxy)
			s.cfg.BindAddr = "127.0.0.1:19999"
			r := httptest.NewRequest("GET", "/apply", nil)
			r.RemoteAddr = tc.proxy + ":41000"
			r.Header.Set("X-Forwarded-For", tc.client)

			v := s.reachVerdict(r, shared.Rules{TCP: []shared.PortRule{{Port: tc.open}}},
				shared.FirewallOptions{}, shared.NetworkSettings{})
			if v.Verdict != shared.ReachUnknown || v.Reason != shared.ReasonTrustedProxy {
				t.Errorf("verdict = %s/%s for port %s, want %s/%s — the port the client reaches is the proxy's",
					v.Verdict, v.Reason, v.Port, shared.ReachUnknown, shared.ReasonTrustedProxy)
			}
			if v.Addr != tc.client {
				t.Errorf("verdict address = %q, want the resolved client %q", v.Addr, tc.client)
			}
		})
	}
}
