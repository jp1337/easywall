package web

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
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

// pinnedTLSConfig builds a tls.Config that accepts exactly the certificate
// this installation serves — the file certPath names, which is the same one
// certManager.GetCertificate loads for every real handshake — and refuses
// anything else.
//
// RootCAs holds only that one certificate, so the (always-run; nothing here
// sets InsecureSkipVerify) chain check succeeds for it and for nothing else:
// a different process answering on the same port has a different keypair and
// cannot build a chain into this pool, self-signed or not.
//
// ServerName is set to a name the certificate itself lists as a Subject
// Alternative Name, so hostname verification passes against this exact leaf
// regardless of which address was actually dialled — a health check runs
// against this installation's own bind_addr, which may be an address the
// certificate was never issued for (an operator's LAN IP, say, against
// easywall's generated cert, which only ever names localhost/127.0.0.1/::1).
// Picking a SAN the certificate already carries, rather than the dialled
// host, is what makes that work without weakening the check: the SAN is
// still checked against the one pinned certificate, just not against the
// network address, which pinning has already made redundant.
//
// A certificate with no SAN at all — a CN-only certificate no modern
// verifier accepts on the wire in the first place — leaves ServerName empty,
// which skips hostname verification and keeps only the chain check above:
// still exactly this certificate, never InsecureSkipVerify.
func pinnedTLSConfig(certPath string) (*tls.Config, error) {
	// #nosec G304 -- certPath is Config.CertPath(): ssl_dir or tls.cert from
	// the config this process was started with, never a request.
	data, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read TLS certificate %s: %w", certPath, err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM certificate found in %s", certPath)
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse TLS certificate %s: %w", certPath, err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(leaf)

	var serverName string
	switch {
	case len(leaf.DNSNames) > 0:
		serverName = leaf.DNSNames[0]
	case len(leaf.IPAddresses) > 0:
		serverName = leaf.IPAddresses[0].String()
	}
	return &tls.Config{RootCAs: pool, ServerName: serverName}, nil
}

// HealthCheck asks this installation's /healthz and returns nil on a 200.
// cfg comes from LoadConfig alone: Validate writes a session key, and a probe
// that runs every ten seconds as root must write nothing.
//
// No proxy, whatever the environment says: the target is this host. The
// certificate this process itself serves (Config.CertPath) is pinned — see
// pinnedTLSConfig — so what is being checked is both that the process
// answers and that it is still the one process this configuration names.
func HealthCheck(cfg *Config) error {
	url, host, err := healthTarget(cfg.BindAddr)
	if err != nil {
		return err
	}
	tlsConfig, err := pinnedTLSConfig(cfg.CertPath())
	if err != nil {
		return fmt.Errorf("health check %s: %w", url, err)
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
			Proxy:           nil,
			DialContext:     dialer.DialContext,
			TLSClientConfig: tlsConfig,
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
