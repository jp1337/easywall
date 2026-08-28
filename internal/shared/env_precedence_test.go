package shared

import (
	"os"
	"path/filepath"
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
		for _, v := range WebEnvVars {
			t.Run(v.Name, func(t *testing.T) {
				var cfg WebConfig
				if _, err := toml.DecodeFile(path, &cfg); err != nil {
					t.Fatal(err)
				}
				raw := sampleValue(v.Kind, "web-"+v.TOMLKey)
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
		for _, v := range CoreEnvVars {
			t.Run(v.Name, func(t *testing.T) {
				var cfg CoreConfig
				if _, err := toml.DecodeFile(path, &cfg); err != nil {
					t.Fatal(err)
				}
				raw := sampleValue(v.Kind, "core-"+v.TOMLKey)
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
			raw := sampleValue(v.Kind, "rt-"+v.TOMLKey)
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
			raw := sampleValue(v.Kind, "rt-"+v.TOMLKey)
			if err := v.Set(&cfg, raw); err != nil {
				t.Fatalf("Set(%q): %v", raw, err)
			}
			if got := v.Get(&cfg); got != raw {
				t.Errorf("Set(%q) then Get = %q; the pair describes different fields", raw, got)
			}
		})
	}
}

// sampleValue produces a value of the right shape that no default could be. The
// booleans are inverted from every shipped default on purpose — "false" for a
// key shipping true and back again would prove nothing.
func sampleValue(kind EnvKind, seed string) string {
	if kind == EnvBool {
		return "true"
	}
	return "/from-the-environment/" + seed
}
