package shared

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/jp1337/easywall/config"
)

// The requirement the whole release turns on, in the form the spec states it:
// write the shipped configuration to a file, set a variable whose key that file
// already contains at its default, and the variable still wins.
//
// `easywall-web -write-config` emits every default, and that file is what the
// container image ships. Under the naive reading of "stored" — the key is
// present — such a file would silently disable every environment variable in the
// product, on exactly the installations that use them.
//
// Driven from the tables rather than from a list of keys, so a fourteenth
// variable is covered the day it is added.
func TestAShippedDefaultDoesNotCountAsStored(t *testing.T) {
	t.Run("web", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "web.toml")
		if err := os.WriteFile(path, config.Web, 0600); err != nil {
			t.Fatal(err)
		}
		def := WebDefault()
		for _, v := range WebEnvVars {
			t.Run(v.Name, func(t *testing.T) {
				var cfg WebConfig
				if _, err := toml.DecodeFile(path, &cfg); err != nil {
					t.Fatal(err)
				}
				raw := sampleValue(v.Kind, "web-"+v.TOMLKey, v.Get(&def))
				prov, err := applyEnv(&cfg, WebDefault(), WebEnvVars,
					lookup(map[string]string{v.Name: raw}))
				if err != nil {
					t.Fatalf("applyEnv: %v", err)
				}
				if got := v.Get(&cfg); got != raw {
					t.Errorf("%s = %q; the shipped file's default beat the environment's %q\n"+
						"  every containerised installation sets its variables against a file "+
						"that states every default", v.TOMLKey, got, raw)
				}
				if p := prov[v.TOMLKey]; p.Overridden() {
					t.Errorf("%s is reported as overridden by %q, and nobody stored it",
						v.TOMLKey, p.Stored)
				}
			})
		}
	})

	t.Run("core", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "easywall.toml")
		if err := os.WriteFile(path, config.Core, 0600); err != nil {
			t.Fatal(err)
		}
		def := CoreDefault()
		for _, v := range CoreEnvVars {
			t.Run(v.Name, func(t *testing.T) {
				var cfg CoreConfig
				if _, err := toml.DecodeFile(path, &cfg); err != nil {
					t.Fatal(err)
				}
				raw := sampleValue(v.Kind, "core-"+v.TOMLKey, v.Get(&def))
				if _, err := applyEnv(&cfg, CoreDefault(), CoreEnvVars,
					lookup(map[string]string{v.Name: raw})); err != nil {
					t.Fatalf("applyEnv: %v", err)
				}
				if got := v.Get(&cfg); got != raw {
					t.Errorf("%s = %q, want the environment's %q", v.TOMLKey, got, raw)
				}
			})
		}
	})
}

// A value the operator actually stored beats the variable, which is the other
// half of the same rule and the half that is a behaviour change.
func TestAStoredValueBeatsTheEnvironment(t *testing.T) {
	cfg := WebDefault()
	cfg.Language = "de" // the shipped file says "en"; this is an operator's edit

	prov, err := applyEnv(&cfg, WebDefault(), WebEnvVars,
		lookup(map[string]string{"EASYWALL_WEB_LANGUAGE": "fr"}))
	if err != nil {
		t.Fatalf("applyEnv: %v", err)
	}
	if cfg.Language != "de" {
		t.Errorf("Language = %q, want the stored de", cfg.Language)
	}
	p := prov["language"]
	if !p.Overridden() {
		t.Errorf("provenance for language = %+v, want it reported as overridden", p)
	}
	if p.Env != "fr" || p.Stored != "de" || p.Name != "EASYWALL_WEB_LANGUAGE" {
		t.Errorf("provenance for language = %+v, want {EASYWALL_WEB_LANGUAGE fr de}", p)
	}
}

// A stored value that happens to equal the variable is not a conflict, and the
// interface must not draw one.
func TestAStoredValueEqualToTheEnvironmentIsNotAConflict(t *testing.T) {
	cfg := WebDefault()
	cfg.Language = "fr"

	prov, err := applyEnv(&cfg, WebDefault(), WebEnvVars,
		lookup(map[string]string{"EASYWALL_WEB_LANGUAGE": "fr"}))
	if err != nil {
		t.Fatalf("applyEnv: %v", err)
	}
	if p := prov["language"]; p.Overridden() {
		t.Errorf("provenance for language = %+v, want no conflict when the two agree", p)
	}
}

// A variable nobody can parse stops the process whether or not a stored value
// would have beaten it. An operator who later removes the stored value should
// not discover the typo then.
func TestAnUnparseableBoolFailsEvenWhenItWouldHaveLost(t *testing.T) {
	cfg := WebDefault()
	yes := true
	cfg.UpdateCheck = &yes
	cfg.Language = "de" // make something else stored too, so the config is realistic

	no := false
	cfg.UpdateCheck = &no // the shipped file says true; this is stored

	if _, err := applyEnv(&cfg, WebDefault(), WebEnvVars,
		lookup(map[string]string{"EASYWALL_WEB_UPDATE_CHECK": "yes"})); err == nil {
		t.Fatal("want an error for EASYWALL_WEB_UPDATE_CHECK=yes, got nil")
	}
}

// Get and Set are two closures per entry describing one key. A pair that
// disagrees would make "stored" mean nothing: the comparison reads one and the
// overlay writes the other.
func TestEveryEnvVarRoundTripsThroughGetAndSet(t *testing.T) {
	for _, v := range WebEnvVars {
		t.Run(v.Name, func(t *testing.T) {
			cfg := WebDefault()
			raw := sampleValue(v.Kind, "rt-"+v.TOMLKey, v.Get(&cfg))
			if err := v.Set(&cfg, raw); err != nil {
				t.Fatalf("Set(%q): %v", raw, err)
			}
			if got := v.Get(&cfg); got != raw {
				t.Errorf("Set(%q) then Get = %q; the pair describes different fields", raw, got)
			}
		})
	}
	for _, v := range CoreEnvVars {
		t.Run(v.Name, func(t *testing.T) {
			cfg := CoreDefault()
			raw := sampleValue(v.Kind, "rt-"+v.TOMLKey, v.Get(&cfg))
			if err := v.Set(&cfg, raw); err != nil {
				t.Fatalf("Set(%q): %v", raw, err)
			}
			if got := v.Get(&cfg); got != raw {
				t.Errorf("Set(%q) then Get = %q; the pair describes different fields", raw, got)
			}
		})
	}
}

// WebDefault has to hand back a value nobody else's copy can corrupt.
// UpdateCheck and Telemetry are *bool: if two calls returned the same pointer,
// *WebDefault().UpdateCheck = false would reach back into the package-level
// default that every later comparison in the process compares against, and a
// file saying update_check = true would start reading as stored — silently,
// and for the rest of the process's life.
func TestWebDefaultDoesNotAliasThePackageLevelDefault(t *testing.T) {
	a, b := WebDefault(), WebDefault()
	if a.UpdateCheck == nil || b.UpdateCheck == nil {
		t.Fatal("config/web.toml no longer sets update_check; this test needs a non-nil pointer to prove anything")
	}
	if a.UpdateCheck == b.UpdateCheck {
		t.Error("WebDefault().UpdateCheck is the same pointer on every call")
	}

	*a.UpdateCheck = !*a.UpdateCheck
	if *a.UpdateCheck == *b.UpdateCheck {
		t.Error("mutating one WebDefault()'s UpdateCheck changed another's")
	}

	// RecoveryCodes ships empty, so aliasing has nothing to corrupt through the
	// package's own defaults — seed a non-empty slice into the package-level
	// value directly (this test lives in package shared) and restore it after.
	prevCodes := webDefault.RecoveryCodes
	webDefault.RecoveryCodes = []string{"original"}
	t.Cleanup(func() { webDefault.RecoveryCodes = prevCodes })

	got := WebDefault()
	got.RecoveryCodes[0] = "mutated"
	if webDefault.RecoveryCodes[0] != "original" {
		t.Errorf("WebDefault().RecoveryCodes aliases the package-level default: got %q", webDefault.RecoveryCodes[0])
	}

	// Telemetry ships nil since 2.12 — config/web.toml no longer pre-answers
	// consent, so the shipped default has nothing for WebDefault() to clone.
	// Seeded directly into the package-level value, the same way RecoveryCodes
	// is above, so the clone this function makes of a *bool is still exercised.
	prevTelemetry := webDefault.Telemetry
	seed := true
	webDefault.Telemetry = &seed
	t.Cleanup(func() { webDefault.Telemetry = prevTelemetry })

	ta, tb := WebDefault(), WebDefault()
	if ta.Telemetry == nil || tb.Telemetry == nil {
		t.Fatal("seeded webDefault.Telemetry directly; WebDefault() should have cloned a non-nil pointer")
	}
	if ta.Telemetry == tb.Telemetry {
		t.Error("WebDefault().Telemetry is the same pointer on every call")
	}
	*ta.Telemetry = !*ta.Telemetry
	if *ta.Telemetry == *tb.Telemetry {
		t.Error("mutating one WebDefault()'s Telemetry changed another's")
	}
	if webDefault.Telemetry != &seed || *webDefault.Telemetry != true {
		t.Error("mutating a WebDefault()'s Telemetry changed the package-level default")
	}
}

// CoreDefault needs the same guarantee for its two slice fields.
func TestCoreDefaultDoesNotAliasThePackageLevelDefault(t *testing.T) {
	prevCustom := coreDefault.Docker.CustomNetworks
	prevRouting := coreDefault.Routing.Networks
	coreDefault.Docker.CustomNetworks = []string{"original"}
	coreDefault.Routing.Networks = []string{"original"}
	t.Cleanup(func() {
		coreDefault.Docker.CustomNetworks = prevCustom
		coreDefault.Routing.Networks = prevRouting
	})

	got := CoreDefault()
	got.Docker.CustomNetworks[0] = "mutated"
	got.Routing.Networks[0] = "mutated"

	if coreDefault.Docker.CustomNetworks[0] != "original" {
		t.Errorf("CoreDefault().Docker.CustomNetworks aliases the package-level default: got %q",
			coreDefault.Docker.CustomNetworks[0])
	}
	if coreDefault.Routing.Networks[0] != "original" {
		t.Errorf("CoreDefault().Routing.Networks aliases the package-level default: got %q",
			coreDefault.Routing.Networks[0])
	}
}

// sampleValue produces a value of the right shape that differs from def, the
// key's own built-in default rendered the way Get would render it. A value
// equal to the default proves nothing: TestAShippedDefaultDoesNotCountAsStored
// would still pass against an applyEnv mutated to never call Set at all,
// because the field already held that value before Set was ever reached.
// EASYWALL_WEB_UPDATE_CHECK ships true, which is exactly what a hardcoded
// "true" would have collided with — inverting def is what keeps every
// boolean key covered, not just the ones shipping false.
func sampleValue(kind EnvKind, seed, def string) string {
	if kind == EnvBool {
		return strconv.FormatBool(def != "true")
	}
	v := "/from-the-environment/" + seed
	if v == def {
		panic("sampleValue: " + seed + " collides with the shipped default " + def + "; pick a different seed")
	}
	return v
}
