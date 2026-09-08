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

	res, err := s.client.GetHealth()
	if err != nil {
		// The core is unreachable, which is the half this endpoint's sibling
		// check already covers — but a firewall nothing can ask about cannot be
		// shown to be enforcing anything, so it is fail rather than degraded.
		// not_enforcing because HealthReason is a closed enum both locale files
		// label: an "unreachable" value would have to be added there too, and
		// "we cannot show this is enforcing" is what it would say.
		slog.Warn("healthz: core unreachable", "error", err)
		writeHealth(w, http.StatusServiceUnavailable, shared.HealthResult{
			State:  shared.HealthFail,
			Reason: shared.HealthReasonNotEnforcing,
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
func writeHealth(w http.ResponseWriter, code int, res shared.HealthResult) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		slog.Warn("healthz: write response", "error", err)
	}
}
