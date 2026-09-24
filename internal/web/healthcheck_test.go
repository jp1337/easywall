package web

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// pinnedTLSTestServer returns an unstarted httptest.Server presenting the
// certificate this installation's own certPath names — generated fresh into
// s.cfg.SSLDir — so a caller only has to attach a Listener and call
// StartTLS. httptest.Server.StartTLS uses s.TLS.Certificates when they are
// already set rather than minting its own fixed test certificate, which is
// what makes it possible to test HealthCheck's pinning against a server
// presenting the exact certificate it is meant to accept.
func pinnedTLSTestServer(t *testing.T, s *Server) *httptest.Server {
	t.Helper()
	if err := generateSelfSignedCert(s.cfg.SSLDir); err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}
	cert, err := tls.LoadX509KeyPair(s.cfg.CertPath(), s.cfg.KeyPath())
	if err != nil {
		t.Fatalf("load generated cert: %v", err)
	}
	srv := httptest.NewUnstartedServer(s.router)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	return srv
}

func TestHealthTargetFollowsBindAddr(t *testing.T) {
	for bind, want := range map[string]string{
		"0.0.0.0:12227":        "https://127.0.0.1:12227/healthz",
		":12228":               "https://127.0.0.1:12228/healthz",
		"[::]:12227":           "https://127.0.0.1:12227/healthz",
		"192.0.2.5:12228":      "https://192.0.2.5:12228/healthz",
		"[2001:db8::1]:12227":  "https://[2001:db8::1]:12227/healthz",
		"[fe80::1%eth0]:12227": "https://[fe80::1%25eth0]:12227/healthz",
	} {
		if got, _, err := healthTarget(bind); err != nil || got != want {
			t.Errorf("healthTarget(%q) = %q, %v; want %q", bind, got, err, want)
		}
	}
	if _, _, err := healthTarget("no-port"); err == nil {
		t.Error("a bind_addr with no port produced a target")
	}
}

// Fix round 1: net.ParseIP returns nil for a zoned IPv6 literal, which left
// dialer.LocalAddr unset for exactly the bind_addr shape most likely to need
// it. dialFrom must use netip.ParseAddr and carry the zone through.
func TestDialFromPinsAZonedIPv6HostAsTheSource(t *testing.T) {
	got := dialFrom("fe80::1%eth0")
	if got == nil {
		t.Fatal("dialFrom of a zoned host returned nil, want a TCPAddr with the zone")
	}
	if got.Zone != "eth0" {
		t.Errorf("Zone = %q, want %q", got.Zone, "eth0")
	}
	want := net.ParseIP("fe80::1")
	if !got.IP.Equal(want) {
		t.Errorf("IP = %v, want %v", got.IP, want)
	}

	if dialFrom("") != nil {
		t.Error("dialFrom(\"\") should leave the source to the kernel, not pin to a zero address")
	}
}

// The 2.22 finding C, end to end: a bind on one specific address refuses
// 127.0.0.1, and health_allow does not list that address. -healthcheck must
// still read healthy — and read unhealthy when the core says fail.
func TestHealthCheckAsksASpecificBind(t *testing.T) {
	for state, wantOK := range map[shared.HealthState]bool{shared.HealthOK: true, shared.HealthFail: false} {
		t.Run(string(state), func(t *testing.T) {
			fc := newFakeCore(t)
			fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{State: state}))
			s := newTestServer(t, fc)
			// Loopback is not on it: only the own-address rule can admit 127.0.0.2.
			s.cfg.HealthAllow = []string{"::1/128"}

			ln, err := net.Listen("tcp", "127.0.0.2:0")
			if err != nil {
				t.Skipf("cannot bind 127.0.0.2 here: %v", err)
			}
			srv := pinnedTLSTestServer(t, s)
			srv.Listener = ln
			srv.StartTLS()
			t.Cleanup(srv.Close)
			s.cfg.BindAddr = ln.Addr().String()

			err = HealthCheck(s.cfg)
			if wantOK && err != nil {
				t.Fatalf("a healthy installation bound to %s read unhealthy: %v", s.cfg.BindAddr, err)
			}
			if !wantOK && (err == nil || !strings.Contains(err.Error(), "503")) {
				t.Fatalf("a failing firewall read %v, want a 503 error", err)
			}
		})
	}

	// Fix round 2, end to end: a zoned bind_addr on a link-local address.
	// Skipped where this host has none to bind.
	t.Run("zoned", func(t *testing.T) {
		ip, zone := linkLocalIPv6WithZone(t)
		fc := newFakeCore(t)
		fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{State: shared.HealthOK}))
		s := newTestServer(t, fc)
		// Loopback and every non-link-local network are not on it: only the
		// own-address rule can admit this zoned peer.
		s.cfg.HealthAllow = []string{"::1/128"}

		ln, err := net.Listen("tcp6", "["+ip+"%"+zone+"]:0")
		if err != nil {
			t.Skipf("cannot bind [%s%%%s] here: %v", ip, zone, err)
		}
		srv := pinnedTLSTestServer(t, s)
		srv.Listener = ln
		srv.StartTLS()
		t.Cleanup(srv.Close)
		// Not ln.Addr().String(): Go's TCPListener drops the zone from the
		// address getsockname(2) hands back, so the listener's own address
		// string reads as though the bind had no zone at all. bind_addr in a
		// real config carries the zone the operator wrote, so it is put back
		// here from what linkLocalIPv6WithZone found — only the port comes
		// from the listener.
		_, port, err := net.SplitHostPort(ln.Addr().String())
		if err != nil {
			t.Fatalf("listener address %q has no port: %v", ln.Addr(), err)
		}
		s.cfg.BindAddr = net.JoinHostPort(ip+"%"+zone, port)

		if err := HealthCheck(s.cfg); err != nil {
			t.Fatalf("a healthy installation bound to a zoned address %s read unhealthy: %v", s.cfg.BindAddr, err)
		}
	})
}

// linkLocalIPv6WithZone finds a link-local IPv6 address and the interface
// name (the zone) it lives on, or skips the test — a host with no such
// interface (a CI runner with only loopback and a plain IPv4 bridge, say)
// cannot exercise a zoned bind at all.
func linkLocalIPv6WithZone(t *testing.T) (ip, zone string) {
	t.Helper()
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Skipf("cannot list interfaces: %v", err)
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.To4() != nil || !ipnet.IP.IsLinkLocalUnicast() {
				continue
			}
			return ipnet.IP.String(), iface.Name
		}
	}
	t.Skip("no link-local IPv6 address found on this host")
	return "", ""
}

// The own-address rule admits exactly one peer, and not at all when the
// operator has switched the endpoint off with an empty list.
func TestHealthzAdmitsItsOwnAddressAndNoOther(t *testing.T) {
	for _, tc := range []struct {
		name, peer, local, zone string
		allow                   []string
		want                    int
	}{
		{"own address", "10.0.0.9:40000", "10.0.0.9", "", []string{"127.0.0.1/8"}, http.StatusOK},
		{"a neighbour", "10.0.0.5:40000", "10.0.0.9", "", []string{"127.0.0.1/8"}, http.StatusNotFound},
		{"own address, endpoint off", "10.0.0.9:40000", "10.0.0.9", "", []string{}, http.StatusNotFound},
		// The own-address rule is not scoped to loopback: a same-host process
		// (a reverse proxy relaying remote traffic, say) reaches the bound
		// address as its own peer and is admitted even though the list names
		// neither 127.0.0.1 nor that address. Only [] closes the endpoint.
		{"loopback, list without it", "127.0.0.1:40000", "127.0.0.1", "", []string{"10.0.0.0/8"}, http.StatusOK},
		// Fix round 2: a zoned bind_addr. Go's net stack writes a zone onto a
		// link-local peer's RemoteAddr; localIP used to drop the zone from the
		// local side, so a zoned peer and its own zoned local address never
		// compared equal and the own-address rule could never match at all.
		{"own address, zoned", "[fe80::9%eth0]:40000", "fe80::9", "eth0", []string{"::1/128"}, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fc := newFakeCore(t)
			fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{State: shared.HealthOK}))
			s := newTestServer(t, fc)
			s.cfg.HealthAllow = tc.allow

			req := httptest.NewRequest("GET", "/healthz", nil)
			req.RemoteAddr = tc.peer
			req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey,
				&net.TCPAddr{IP: net.ParseIP(tc.local), Zone: tc.zone, Port: 12227}))
			rec := httptest.NewRecorder()
			s.router.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("answered %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

// The 2.22 CodeQL finding (go/disabled-certificate-check): HealthCheck used
// to skip certificate verification entirely. A server presenting a
// different certificate — what a man-in-the-middle, or another process that
// merely took over the same port, would present — must be refused. This
// test goes red if InsecureSkipVerify comes back, or if pinning is loosened
// to accept any certificate the peer happens to offer.
func TestHealthCheckRefusesADifferentCertificate(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{State: shared.HealthOK}))
	s := newTestServer(t, fc)
	s.cfg.HealthAllow = []string{"127.0.0.1/32"}

	// The certificate this installation is configured to pin: generated once
	// into its own ssl_dir, the way production does it.
	if err := generateSelfSignedCert(s.cfg.SSLDir); err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}

	// The certificate the server under test actually presents: a different
	// one, generated into a directory of its own.
	otherDir := t.TempDir()
	if err := generateSelfSignedCert(otherDir); err != nil {
		t.Fatalf("generateSelfSignedCert (other): %v", err)
	}
	otherCert, err := tls.LoadX509KeyPair(otherDir+"/cert.pem", otherDir+"/key.pem")
	if err != nil {
		t.Fatalf("load other cert: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := httptest.NewUnstartedServer(s.router)
	srv.Listener = ln
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{otherCert}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	s.cfg.BindAddr = ln.Addr().String()

	if err := HealthCheck(s.cfg); err == nil {
		t.Fatal("HealthCheck accepted a server presenting a different certificate")
	}
}

// A missing certificate file is a startup-shaped failure, not a passed
// health check: HealthCheck must return an error rather than silently
// skipping the pin (which is what InsecureSkipVerify amounted to).
func TestHealthCheckErrorsOnMissingCertFile(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{State: shared.HealthOK}))
	s := newTestServer(t, fc)
	s.cfg.HealthAllow = []string{"127.0.0.1/32"}
	// s.cfg.SSLDir exists (newTestServer creates it) but nothing has written
	// cert.pem into it: CertPath names a file that is not there.

	// A real, reachable, healthy server all the same — presenting some
	// certificate of its own — so a HealthCheck that returned nil here could
	// only have done so by skipping the pin instead of erroring on the
	// missing file, not because there was nothing to reach.
	otherDir := t.TempDir()
	if err := generateSelfSignedCert(otherDir); err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}
	otherCert, err := tls.LoadX509KeyPair(otherDir+"/cert.pem", otherDir+"/key.pem")
	if err != nil {
		t.Fatalf("load cert: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := httptest.NewUnstartedServer(s.router)
	srv.Listener = ln
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{otherCert}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	s.cfg.BindAddr = ln.Addr().String()

	if err := HealthCheck(s.cfg); err == nil {
		t.Fatal("HealthCheck with no certificate file on disk returned nil, want an error")
	}
}
