package web

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/jp1337/easywall/internal/shared"
)

// Who a request is from.
//
// Until 2.13 the answer was one line: the TCP peer, always, because a
// forwarding header is written by whoever is on the other end of the socket and
// easywall-web is not assumed to sit behind anything. That is still the answer
// when nothing is configured, and the empty list is the default —
// TestTheEmptyListIsTwoPointTwelve holds the two paths byte-identical.
//
// What 2.13 adds is a *list* of peers whose header is believed. Never a
// boolean: "trust the header" with no way to say whose is exactly
// GHSA-3fxj-6jh8-hvhx, GHSA-rjr7-jggh-pgcp and GHSA-9g5q-2w5x-hmxf, and
// docs-tech/threat-model.md carries the argument in full.

// proxyHeaders are the headers whose *presence* means the peer is not the
// client. Only X-Forwarded-For's value is ever read — see resolveClient.
var proxyHeaders = []string{"X-Forwarded-For", "X-Real-IP", "True-Client-IP", "Forwarded"}

// peerIP is the TCP peer address, and only the peer address.
func peerIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// proxiedRequest reports whether this request arrived through something that
// forwards, so the interface can say that the address it recorded is not the
// caller's.
//
// Presence only, and it is consulted only when the peer is *not* trusted. A
// client that forges a header from an untrusted peer can move its own verdict
// to "cannot tell" and its own login line to via-proxy, and achieve nothing
// else: the recorded address is still the peer's. That asymmetry is the whole
// argument for reading an untrusted header at all.
func proxiedRequest(r *http.Request) bool {
	for _, h := range proxyHeaders {
		if _, ok := r.Header[http.CanonicalHeaderKey(h)]; ok {
			return true
		}
	}
	return false
}

// resolveClient answers who this request is from, and whether that address is a
// stand-in for somebody it cannot name.
//
// The peer decides which of the two branches runs, and the peer is the one
// thing on a request nobody can write: it comes from the kernel's copy of the
// socket. If it is on the list, X-Forwarded-For is read; if it is not, the
// header's value is never looked at, exactly as before 2.13.
//
// On the trusted branch, proxied is false only when the walk named a client —
// an address the header actually supplied. It is true whenever the walk falls
// back to the peer, because the peer is then standing in for somebody the
// walk could not name: no usable header, or a caller who forged the trusted
// proxy's own address and exhausted the chain. Marking that case via-proxy is
// what tells the interface the address is the proxy, not a confirmed client —
// exactly the shape a `trusted_proxies` entry without a header-setting reverse
// proxy produces, and the case this release must not hide.
func resolveClient(r *http.Request, trusted []string) (string, bool) {
	peer := peerIP(r)
	addr, err := netip.ParseAddr(peer)
	if err != nil || !shared.InAnyEntry(addr.Unmap(), trusted) {
		return peer, proxiedRequest(r)
	}
	client := rightmostUntrusted(r, trusted, peer)
	return client, client == peer
}

// rightmostUntrusted walks X-Forwarded-For from the right and returns the first
// hop that is not itself a trusted proxy.
//
// From the right, because the chain is appended to: everything to the left of a
// hop was written by whoever was talking to that hop, and the caller controls
// the left-hand end completely. Walking past trusted entries is what stops a
// caller writing "X-Forwarded-For: 10.1.0.5" — a proxy's own address — and
// being handed that identity; the veth test's third case is exactly that.
//
// X-Forwarded-For alone. X-Real-IP and True-Client-IP carry one address with no
// hop semantics, and Forwarded (RFC 7239) is a different grammar whose parser
// would be a second surface for no gain. All four still mark an untrusted
// request through proxiedRequest.
//
// An entry that is not an address ends the walk at the peer rather than being
// skipped: everything left of it was written by whatever put it there, and a
// hop that is not an address is not one this can reason past.
func rightmostUntrusted(r *http.Request, trusted []string, peer string) string {
	var hops []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(v, ",") {
			hops = append(hops, strings.TrimSpace(part))
		}
	}
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(hops[i])
		if err != nil {
			return peer
		}
		if a := addr.Unmap(); !shared.InAnyEntry(a, trusted) {
			return a.String()
		}
	}
	return peer
}
