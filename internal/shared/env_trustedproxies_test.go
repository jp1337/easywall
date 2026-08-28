package shared

import (
	"net/netip"
	"testing"
)

// The variable is a list, and the list is the only shape this setting has.
// A boolean here is the vulnerability GHSA-3fxj-6jh8-hvhx, GHSA-rjr7-jggh-pgcp
// and GHSA-9g5q-2w5x-hmxf describe, so the kind is asserted, not just the value.
func TestTrustedProxiesComesFromTheEnvironmentAsAList(t *testing.T) {
	v, ok := WebEnvVar("trusted_proxies")
	if !ok {
		t.Fatal("no environment entry names trusted_proxies")
	}
	if v.Name != "EASYWALL_WEB_TRUSTED_PROXIES" {
		t.Errorf("variable is %q, want EASYWALL_WEB_TRUSTED_PROXIES", v.Name)
	}
	if v.Kind != EnvList {
		t.Errorf("kind is %v, want EnvList — a boolean here is the advisory", v.Kind)
	}

	cfg := WebDefault()
	if err := v.Set(&cfg, "10.1.0.5, 192.0.2.0/24 ,"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	want := []string{"10.1.0.5", "192.0.2.0/24"}
	if len(cfg.TrustedProxies) != len(want) {
		t.Fatalf("TrustedProxies = %v, want %v", cfg.TrustedProxies, want)
	}
	for i := range want {
		if cfg.TrustedProxies[i] != want[i] {
			t.Errorf("entry %d = %q, want %q", i, cfg.TrustedProxies[i], want[i])
		}
	}
}

// A typo stops startup rather than silently shortening the list. A list one
// entry short stops trusting a proxy that is really there, and the operator
// meets that as a rate limiter counting everyone together — which is not a
// diagnosis anybody makes.
func TestAnUnparseableProxyEntryIsRefused(t *testing.T) {
	cfg := WebDefault()
	_, err := applyEnv(&cfg, WebDefault(), WebEnvVars,
		lookup(map[string]string{"EASYWALL_WEB_TRUSTED_PROXIES": "10.1.0.5,not-an-address"}))
	if err == nil {
		t.Fatal("want an error for a malformed entry, got nil")
	}
}

// The shipped default trusts nobody. Every other guarantee in 2.13 rests on
// this one: an installation that does not configure the list behaves exactly
// as 2.12 did.
func TestTheShippedDefaultTrustsNobody(t *testing.T) {
	if got := WebDefault().TrustedProxies; len(got) != 0 {
		t.Errorf("config/web.toml ships trusted_proxies = %v, want an empty list", got)
	}
}

// The matcher the resolution uses is the one the rule builders use. A second
// address matcher is how the list check and the rule check come to disagree.
func TestInAnyEntryMatchesAddressesAndNetworks(t *testing.T) {
	list := []string{"# the front proxy", "10.1.0.5", "192.0.2.0/24", ""}
	for _, tc := range []struct {
		addr string
		want bool
	}{
		{"10.1.0.5", true},
		{"10.1.0.6", false},
		{"192.0.2.99", true},
		{"198.51.100.1", false},
	} {
		if got := InAnyEntry(netip.MustParseAddr(tc.addr), list); got != tc.want {
			t.Errorf("InAnyEntry(%s) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}
