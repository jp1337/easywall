package web

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHealthTargetFollowsBindAddr(t *testing.T) {
	for bind, want := range map[string]string{
		"0.0.0.0:12227":       "https://127.0.0.1:12227/healthz",
		":12228":              "https://127.0.0.1:12228/healthz",
		"[::]:12227":          "https://127.0.0.1:12227/healthz",
		"192.0.2.5:12228":     "https://192.0.2.5:12228/healthz",
		"[2001:db8::1]:12227": "https://[2001:db8::1]:12227/healthz",
	} {
		if got, _, err := healthTarget(bind); err != nil || got != want {
			t.Errorf("healthTarget(%q) = %q, %v; want %q", bind, got, err, want)
		}
	}
	if _, _, err := healthTarget("no-port"); err == nil {
		t.Error("a bind_addr with no port produced a target")
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
			srv := httptest.NewUnstartedServer(s.router)
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
}

// The own-address rule admits exactly one peer, and not at all when the
// operator has switched the endpoint off with an empty list.
func TestHealthzAdmitsItsOwnAddressAndNoOther(t *testing.T) {
	for _, tc := range []struct {
		name, peer, local string
		allow             []string
		want              int
	}{
		{"own address", "10.0.0.9:40000", "10.0.0.9", []string{"127.0.0.1/8"}, http.StatusOK},
		{"a neighbour", "10.0.0.5:40000", "10.0.0.9", []string{"127.0.0.1/8"}, http.StatusNotFound},
		{"own address, endpoint off", "10.0.0.9:40000", "10.0.0.9", []string{}, http.StatusNotFound},
		// The own-address rule is not scoped to loopback: a same-host process
		// (a reverse proxy relaying remote traffic, say) reaches the bound
		// address as its own peer and is admitted even though the list names
		// neither 127.0.0.1 nor that address. Only [] closes the endpoint.
		{"loopback, list without it", "127.0.0.1:40000", "127.0.0.1", []string{"10.0.0.0/8"}, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fc := newFakeCore(t)
			fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{State: shared.HealthOK}))
			s := newTestServer(t, fc)
			s.cfg.HealthAllow = tc.allow

			req := httptest.NewRequest("GET", "/healthz", nil)
			req.RemoteAddr = tc.peer
			req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey,
				&net.TCPAddr{IP: net.ParseIP(tc.local), Port: 12227}))
			rec := httptest.NewRecorder()
			s.router.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("answered %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
