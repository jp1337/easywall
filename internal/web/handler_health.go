package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/netip"

	"github.com/jp1337/easywall/internal/shared"
)

// GET /healthz — the one route on this process that answers without a session.
//
// It exists because docker/entrypoint.sh:12-18 records a container that stayed
// "Up" for hours with a live core and a dead web process: easywall-web could
// not write its own config, exited 1, supervisord restarted it for ever, and
// the health check reported the container healthy the whole time — because it
// only looked at the core's socket. A check on the core cannot see this half.
//
// An orchestrator holds no session, so this route holds none either, and it
// therefore starts closed to everything but loopback — see WebConfig.HealthAllow.
//
// The gate reads the TCP peer and never a forwarding header. 2.13's
// resolveClient walk exists so the audit log can record who logged in; it is
// not an authorisation mechanism, and reusing it here would let anything behind
// a trusted proxy read this endpoint by writing "X-Forwarded-For: 127.0.0.1".
// docs-tech/threat-model.md carries that argument in full.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	// 404 and not 403. Not confirming that the endpoint exists costs nothing,
	// because whoever is allowed gets the real answer either way.
	if !healthAllowed(r, s.cfg.HealthAllow) {
		http.NotFound(w, r)
		return
	}

	// GetHealth is not guaranteed to return quickly: both of its netlink reads
	// take the nft mutex, and a slow custom-rules apply can hold this call for
	// up to NftTimeout. See shared.CmdGetHealth for the honest scope and why
	// the five-second protocol deadline is still the right one anyway.
	res, err := s.client.GetHealth()
	if err != nil {
		// core_unreachable and not not_enforcing, and the distinction is the
		// point: not_enforcing is a claim about the kernel, which this process
		// has no path to. All it knows is that it asked and got no answer. The
		// kernel may still be filtering perfectly behind a core that crashed,
		// and reporting it unfiltered would be a false statement — published to
		// anyone who can reach the endpoint, in the release whose subject is not
		// making false claims about the firewall.
		//
		// fail and 503 all the same: a live web process in front of a dead core
		// is the half-dead container docker/entrypoint.sh:12-18 records, and an
		// orchestrator must restart it. The reason changes what the operator is
		// told, not what the orchestrator does.
		slog.Warn("healthz: core unreachable", "error", err)
		writeHealth(w, http.StatusServiceUnavailable, shared.HealthResult{
			State:  shared.HealthFail,
			Reason: shared.HealthReasonCoreUnreachable,
		})
		return
	}

	// degraded answers 200, deliberately, and this is the decision most likely
	// to be "fixed" by a later reader: a degraded firewall is still filtering,
	// and a Docker HEALTHCHECK that marked the container unhealthy for it would
	// restart a working firewall — losing every established connection through
	// it to make a state nobody was in danger from go away. The state is in the
	// body for whoever wants to alert on it, which is where an alert belongs.
	//
	// fail is 503, and so is a core that does not answer.
	code := http.StatusOK
	if res.State == shared.HealthFail {
		code = http.StatusServiceUnavailable
	}
	writeHealth(w, code, *res)
}

// healthAllowed matches the TCP peer — and only the TCP peer — against the
// list. peerIP is the helper that reads r.RemoteAddr and nothing else;
// resolveClient is the one that must not be used here.
func healthAllowed(r *http.Request, allow []string) bool {
	addr, err := netip.ParseAddr(peerIP(r))
	if err != nil {
		return false
	}
	return shared.InAnyEntry(addr.Unmap(), allow)
}

// writeHealth answers with the result as JSON. no-store because a cached "ok"
// is the one answer a health check must never be given.
//
// The kernel release is dropped here, and the reduction is at the endpoint
// rather than in the type on purpose: the *dashboard* reads the same reply over
// the authenticated path and renders it as a fact worth having, so the field
// stays on shared.HealthSelftest and the exposure is closed where the exposure
// is.
//
// HealthSelftest exists because this route is unauthenticated by necessity, and
// its own comment argues that at length — for stripping Detail, which names a
// port number. The reduction stopped one field short. A measured reply from a
// live container read "kernel":"7.2.3-ogc3.1.fc44.x86_64": the host's exact
// release, published with no credential. Both documented health_allow examples
// widen the list to a /24, so an operator following the documentation hands a
// whole subnet the one fact every hardening guide exists to hide — strictly
// more useful to an attacker than the string this type was reduced to exclude.
func writeHealth(w http.ResponseWriter, code int, res shared.HealthResult) {
	res.Selftest.Kernel = ""
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		slog.Warn("healthz: write response", "error", err)
	}
}
