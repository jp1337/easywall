package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// request builds a request with a peer address and any number of headers,
// given as name/value pairs.
func request(t *testing.T, peer string, headers ...string) *http.Request {
	t.Helper()
	if len(headers)%2 != 0 {
		t.Fatalf("headers come in pairs, got %d values", len(headers))
	}
	r := httptest.NewRequest("POST", "/login", nil)
	r.RemoteAddr = peer
	for i := 0; i < len(headers); i += 2 {
		r.Header.Add(headers[i], headers[i+1])
	}
	return r
}

// The empty list is 2.12, byte for byte. Every other guarantee in this release
// rests on an installation that configures nothing behaving as it always did,
// so this is a test and not an intention.
func TestTheEmptyListIsTwoPointTwelve(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    *http.Request
	}{
		{"no header", request(t, "203.0.113.7:5555")},
		{"a forwarded-for header", request(t, "203.0.113.7:5555",
			"X-Forwarded-For", "10.9.9.9")},
		{"a real-ip header", request(t, "203.0.113.7:5555",
			"X-Real-IP", "10.9.9.9")},
		{"several hops", request(t, "203.0.113.7:5555",
			"X-Forwarded-For", "10.9.9.9, 10.8.8.8")},
		{"a peer with no port", request(t, "203.0.113.7")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addr, proxied := resolveClient(tc.r, nil)
			if addr != peerIP(tc.r) {
				t.Errorf("address = %q, want the peer %q", addr, peerIP(tc.r))
			}
			if proxied != proxiedRequest(tc.r) {
				t.Errorf("proxied = %v, want %v", proxied, proxiedRequest(tc.r))
			}
		})
	}
}

// A header from a peer that is not on the list changes nothing at all — spec
// §4.1, at the unit level. The veth test proves the same thing against a real
// kernel-assigned peer address.
func TestAnUntrustedPeerCannotChooseItsAddress(t *testing.T) {
	trusted := []string{"10.1.0.5"}
	r := request(t, "203.0.113.7:5555", "X-Forwarded-For", "198.51.100.1")

	addr, proxied := resolveClient(r, trusted)
	if addr != "203.0.113.7" {
		t.Errorf("address = %q, want the peer 203.0.113.7 — the header was believed", addr)
	}
	if !proxied {
		t.Error("proxied = false; an untrusted header's presence still marks the request")
	}
}

// A header from a peer on the list resolves to the client, and the request is
// no longer marked via-proxy: the recorded address is the caller's, so the
// marker would be saying the opposite of what is true.
func TestATrustedPeerResolvesToTheClient(t *testing.T) {
	trusted := []string{"10.1.0.5"}
	r := request(t, "10.1.0.5:41000", "X-Forwarded-For", "198.51.100.1")

	addr, proxied := resolveClient(r, trusted)
	if addr != "198.51.100.1" {
		t.Errorf("address = %q, want 198.51.100.1", addr)
	}
	if proxied {
		t.Error("proxied = true; the address recorded is the client's, not a stand-in")
	}
}

// The rightmost-untrusted rule: naming a trusted address in the header must not
// let the caller pick its own identity. Spec §4.3 — the bypass the three
// advisories describe.
func TestTheCallerCannotNameATrustedProxyAsItself(t *testing.T) {
	trusted := []string{"10.1.0.5", "10.1.0.6"}
	for _, tc := range []struct {
		name, header, want string
	}{
		{"a trusted hop is walked past", "198.51.100.1, 10.1.0.6", "198.51.100.1"},
		{"every hop is trusted", "10.1.0.6, 10.1.0.5", "10.1.0.5"},
		{"two headers, walked as one chain", "198.51.100.1", "198.51.100.1"},
		// Everything left of a hop was written by whoever was talking to that
		// hop, and the caller controls the left-hand end completely: it can
		// prepend anything. Walking from the left would hand the caller
		// exactly the identity it wrote for itself; walking from the right
		// finds the real client — what the trusted proxy appended — first.
		{"a forged hop to the left of the real one", "203.0.113.99, 198.51.100.1", "198.51.100.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := request(t, "10.1.0.5:41000", "X-Forwarded-For", tc.header)
			if addr, _ := resolveClient(r, trusted); addr != tc.want {
				t.Errorf("address = %q, want %q", addr, tc.want)
			}
		})
	}
}

// A trusted peer sending nothing usable is the peer. Not an error, not empty:
// the proxy is who the connection came from and that is the honest answer.
// proxied is true here, not false: the walk could not name a client, so the
// address returned is the proxy standing in for somebody it cannot name —
// exactly what a `trusted_proxies` entry without a header-setting reverse
// proxy produces, and the interface must say so rather than record it as a
// direct, confirmed login by the proxy host.
func TestATrustedPeerWithNoUsableHeaderIsThePeer(t *testing.T) {
	trusted := []string{"10.1.0.5"}
	for _, tc := range []struct{ name, header string }{
		{"no header at all", ""},
		{"an unparseable hop", "not-an-address"},
		{"an address with a port", "198.51.100.1:443"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := request(t, "10.1.0.5:41000")
			if tc.header != "" {
				r.Header.Set("X-Forwarded-For", tc.header)
			}
			addr, proxied := resolveClient(r, trusted)
			if addr != "10.1.0.5" {
				t.Errorf("address = %q, want the peer 10.1.0.5", addr)
			}
			if !proxied {
				t.Error("proxied = false; the walk named no client, so the peer is a stand-in")
			}
		})
	}
}

// RFC 7230 makes repeated field lines one chain in arrival order: a proxy that
// appends its hop with its own X-Forwarded-For line, rather than folding it
// into the existing one, must not have that hop silently dropped. The
// rightmost hop is therefore in the *last* line, not necessarily the last
// header set on the request.
func TestATrustedPeerWithTwoForwardedForLinesIsOneChain(t *testing.T) {
	r := request(t, "10.1.0.5:41000")
	r.Header.Add("X-Forwarded-For", "203.0.113.99")
	r.Header.Add("X-Forwarded-For", "198.51.100.1")

	addr, _ := resolveClient(r, []string{"10.1.0.5"})
	if addr != "198.51.100.1" {
		t.Errorf("address = %q, want 198.51.100.1 — the second header line was dropped", addr)
	}
}

// The list accepts networks, not only addresses — the same two shapes every
// other address list in the product accepts.
func TestATrustedNetworkCoversItsProxies(t *testing.T) {
	r := request(t, "10.1.0.9:41000", "X-Forwarded-For", "198.51.100.1")
	if addr, _ := resolveClient(r, []string{"10.1.0.0/24"}); addr != "198.51.100.1" {
		t.Errorf("address = %q, want 198.51.100.1", addr)
	}
}

// r.RemoteAddr, never X-Forwarded-For. easywall-web terminates TLS itself and is
// not assumed to sit behind a trusted proxy, so a header would let a client put
// somebody else's address in the firewall's own audit log.
func TestPeerIP_IgnoresForwardingHeaders(t *testing.T) {
	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "203.0.113.7:54321"
	req.Header.Set("X-Forwarded-For", "198.51.100.1")
	req.Header.Set("X-Real-IP", "198.51.100.2")

	if got := peerIP(req); got != "203.0.113.7" {
		t.Errorf("peerIP = %q, want 203.0.113.7 — a header must never reach the audit log", got)
	}
}

// A client can put any of these headers on a request; none of it may ever be
// read except to notice that it is there. This proves the second half of the
// design — a forged header only ever moves the flag to true, never carries a
// value anywhere.
func TestProxiedRequest_ReadsPresenceAndNeverValue(t *testing.T) {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP", "True-Client-IP", "Forwarded"} {
		r := httptest.NewRequest("GET", "/apply", nil)
		r.Header.Set(header, "not-an-address-at-all")
		if !proxiedRequest(r) {
			t.Errorf("%s is present and the request is not reported as proxied", header)
		}
	}
	if proxiedRequest(httptest.NewRequest("GET", "/apply", nil)) {
		t.Error("a request with no forwarding header is not proxied")
	}
}
