package web

import (
	"bytes"
	"log/slog"
	"strings"
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

// The fallback runs before fileConfig is copied, or the default would be
// stored as a file value of false and encode() would write it back.
func TestCountryLookupFallbackPrecedesTheStoredCopy(t *testing.T) {
	cfg, err := LoadConfig(writeTempConfig(t, `bind_addr = "127.0.0.1:1"`))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.fileConfig.CountryLookup {
		t.Error("fileConfig.CountryLookup is false for a file without the key; it would be persisted as off")
	}
}

func TestAKeyInTheWrongTableIsReported(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	if _, err := LoadConfig(writeTempConfig(t, "[tls]\ncountry_lookup = false\n")); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !strings.Contains(buf.String(), "tls.country_lookup") {
		t.Errorf("no warning names tls.country_lookup; log: %q", buf.String())
	}
}
