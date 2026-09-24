package web

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// healthTarget is the URL `easywall-web -healthcheck` asks: /healthz at the
// address this installation binds, with an unspecified host ("0.0.0.0", "::",
// or none) asked on loopback. The container's check used to be a wget fixed at
// 127.0.0.1:12227, which a specific bind refuses and a moved port never
// answers — `unhealthy` for ever on a firewall that was fine.
//
// The host is returned as well: the probe dials from it, see HealthCheck.
func healthTarget(bindAddr string) (url, host string, err error) {
	host, port, err := net.SplitHostPort(bindAddr)
	if err != nil {
		return "", "", fmt.Errorf("bind_addr %q: %w", bindAddr, err)
	}
	if ip, err := netip.ParseAddr(host); host == "" || (err == nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	// A zoned IPv6 literal ("fe80::1%eth0") needs its "%" percent-escaped in a
	// URL, or the request fails with "invalid URL escape". host itself (used
	// to dial from, below) keeps the zone the way net expects it.
	urlHost := strings.ReplaceAll(host, "%", "%25")
	return "https://" + net.JoinHostPort(urlHost, port) + "/healthz", host, nil
}

// dialFrom is the local address a dial from healthTarget's host should bind
// to, or nil to leave the choice to the kernel (host is empty or not a
// parseable address — healthTarget never returns either, so this is only
// reachable if that changes).
//
// netip.ParseAddr, not net.ParseIP: net.ParseIP returns nil for a zoned IPv6
// literal ("fe80::1%eth0"), so a zoned bind_addr silently kept dialer.LocalAddr
// unset and left the source to whatever the route happened to pick — a
// link-local address is exactly the shape most likely to need the zone to be
// routeable at all, so "peer = local by construction" held only by
// coincidence for it.
func dialFrom(host string) *net.TCPAddr {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}
	return &net.TCPAddr{IP: net.IP(addr.AsSlice()), Zone: addr.Zone()}
}

// HealthCheck asks this installation's /healthz and returns nil on a 200.
// cfg comes from LoadConfig alone: Validate writes a session key, and a probe
// that runs every ten seconds as root must write nothing.
//
// No proxy, whatever the environment says: the target is this host. The
// certificate is not verified, because it is this process's own and usually
// self-signed; what is being checked is that the process answers, not who it is.
func HealthCheck(cfg *Config) error {
	url, host, err := healthTarget(cfg.BindAddr)
	if err != nil {
		return err
	}
	// Dialled from the target address itself, so /healthz sees its own address
	// as the peer and admits it (healthAllowed). Left to the kernel, the source
	// is whatever the route says: 127.0.0.2 is reached from 127.0.0.1, and a
	// host with policy routing may pick another address entirely.
	dialer := &net.Dialer{}
	if local := dialFrom(host); local != nil {
		dialer.LocalAddr = local
	}
	client := &http.Client{
		// Under the HEALTHCHECK's own 5s timeout, so a slow answer is reported
		// by this process rather than killed by Docker with no message.
		Timeout: 4 * time.Second,
		Transport: &http.Transport{
			Proxy:       nil,
			DialContext: dialer.DialContext,
			// #nosec G402 -- the peer is this host's own process; see above.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // G402 — see above
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("health check %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check %s: %s", url, resp.Status)
	}
	return nil
}
