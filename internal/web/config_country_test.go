package web

import (
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The lookup ships on (maintainer decision). LoadConfig decodes into a zero
// Config, not into the shipped defaults (the 2.25.0 trap), so an absent key
// would read false; the explicit fallback is what keeps an older web.toml on.
func TestCountryLookupIsOnUnlessSwitchedOff(t *testing.T) {
	if !shared.WebDefault().CountryLookup {
		t.Fatal("config/web.toml ships country_lookup = false; the lookup is on by default")
	}
	load := func(body string) bool {
		t.Helper()
		cfg, err := LoadConfig(writeTempConfig(t, body))
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		return cfg.CountryLookup
	}
	if !load(`bind_addr = "127.0.0.1:1"`) {
		t.Error("a web.toml without country_lookup reads it as off")
	}
	if load("country_lookup = false\n") {
		t.Error("country_lookup = false reads as on")
	}
	t.Setenv("EASYWALL_WEB_COUNTRY_LOOKUP", "false")
	if load(`bind_addr = "127.0.0.1:1"`) {
		t.Error("EASYWALL_WEB_COUNTRY_LOOKUP=false did not switch it off")
	}
}
