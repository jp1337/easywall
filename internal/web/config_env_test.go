package web

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jp1337/easywall/config"
	"github.com/jp1337/easywall/internal/shared"
)

func writeWebConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "web.toml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfig_AStoredValueBeatsTheEnvironment(t *testing.T) {
	path := writeWebConfig(t, "bind_addr = \"127.0.0.1:1111\"\n")
	t.Setenv("EASYWALL_WEB_BIND_ADDR", "0.0.0.0:2222")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.BindAddr != "127.0.0.1:1111" {
		t.Errorf("BindAddr = %q, want the stored 127.0.0.1:1111", cfg.BindAddr)
	}
}

func TestLoadConfig_EnvBeatsTheShippedDefault(t *testing.T) {
	path := writeWebConfig(t, string(config.Web))
	t.Setenv("EASYWALL_WEB_BIND_ADDR", "0.0.0.0:2222")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.BindAddr != "0.0.0.0:2222" {
		t.Errorf("BindAddr = %q, want the environment's 0.0.0.0:2222", cfg.BindAddr)
	}
}

// The overlay has to land before Validate, or an environment value walks past
// every check a file value passes. tls.cert without tls.key is the cheapest
// proof: Validate rejects the pair, and it must reject it identically when the
// half that is set came from the environment.
func TestLoadConfig_EnvValuesAreValidatedLikeFileValues(t *testing.T) {
	path := writeWebConfig(t, "bind_addr = \"127.0.0.1:1111\"\nsocket_path = \"/s.sock\"\nssl_dir = \"/ssl\"\n")
	t.Setenv("EASYWALL_WEB_TLS_CERT", "/only/cert.pem")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	err = cfg.Validate()
	if err == nil {
		t.Fatal("Validate accepted tls.cert with no tls.key")
	}
	if !strings.Contains(err.Error(), "tls.key") {
		t.Errorf("error %q does not name tls.key", err)
	}
}

func TestLoadConfig_UnparseableBoolFailsTheLoad(t *testing.T) {
	path := writeWebConfig(t, "bind_addr = \"127.0.0.1:1111\"\n")
	t.Setenv("EASYWALL_WEB_DEMO_MODE", "sure")

	if _, err := LoadConfig(path); err == nil {
		t.Fatal("LoadConfig accepted EASYWALL_WEB_DEMO_MODE=sure")
	}
}

// The encode() fallback marshals the whole struct. With the overlay applied in
// place, that path would bake an environment value into the operator's file —
// permanently, and with nothing recording where it came from.
func TestEnvOverlayNeverReachesTheConfigFile(t *testing.T) {
	// An empty file is one of the two inputs mergeConfig declines, which is what
	// sends render down the encode() path.
	path := writeWebConfig(t, "")
	t.Setenv("EASYWALL_WEB_BIND_ADDR", "0.0.0.0:31337")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.BindAddr != "0.0.0.0:31337" {
		t.Fatalf("BindAddr = %q; the overlay did not apply, so this test proves nothing",
			cfg.BindAddr)
	}

	// Any Save* takes the same render path. Telemetry is the cheapest.
	if err := cfg.SaveTelemetry(true); err != nil {
		t.Fatalf("SaveTelemetry: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), "0.0.0.0:31337") {
		t.Errorf("the environment value was written to disk:\n%s", written)
	}
	// The write that was actually asked for still has to have happened.
	if !strings.Contains(string(written), "telemetry") {
		t.Errorf("telemetry was not persisted:\n%s", written)
	}
}

// fileSourcedManagedKeys are the managed keys encode()'s copy block takes from
// fileConfig rather than from the live struct. telemetry alone, and for one
// reason: since 2.12, EASYWALL_WEB_TELEMETRY can drive the live value, and
// copying the live value into the file would turn a deployment setting into a
// stored one — which then beats the very variable it came from, permanently,
// from the next password change onwards. fileConfig is what SaveTelemetry and
// SaveFirstRun update, so the operator's own answer still reaches the file;
// TestTelemetryFromTheEnvironmentIsNeverWrittenToTheFile and
// TestSaveTelemetryStillWritesTheOperatorsAnswer
// (config_telemetry_env_test.go) cover this from the other side — the first
// proves the environment's value never lands on disk, the second that the
// operator's answer does.
var fileSourcedManagedKeys = map[string]bool{"telemetry": true}

// TestEncode_CopiesEveryManagedKey guards encode()'s hand-maintained copy
// block — the fifth transcription of managedKeys, after managedValues,
// keyLineRe and sameManagedValues — against falling behind the list it copies
// from. For every name in managedKeys it finds the shared.WebConfig field
// carrying that toml tag, sets only that one field to a value the zero value
// could never produce — on the live struct for most keys, on fileConfig for
// the one in fileSourcedManagedKeys — and insists encode() renders it from
// whichever half the copy block is supposed to read for that key, with the
// other half left zero so nothing else could have supplied the value.
//
// Add a key to managedKeys without adding it to the copy block, and the
// corresponding subtest goes red: the sentinel value never reaches the
// output, because the half it was set on was never copied over. That is
// exactly the failure mode this test exists to catch — a managed value
// silently dropped from a save that falls back to encode(), with the caller
// none the wiser.
func TestEncode_CopiesEveryManagedKey(t *testing.T) {
	typ := reflect.TypeOf(shared.WebConfig{})

	for _, key := range managedKeys {
		t.Run(key, func(t *testing.T) {
			field, ok := tomlField(typ, key)
			if !ok {
				t.Fatalf("no shared.WebConfig field tagged toml:%q", key)
			}

			sentinel, contains := sentinelFor(t, field.Type, key)

			var cfg Config
			if fileSourcedManagedKeys[key] {
				// WebConfig left zero: only fileConfig may supply this value.
				reflect.ValueOf(&cfg.fileConfig).Elem().FieldByIndex(field.Index).Set(sentinel)
			} else {
				// fileConfig left zero: only the live struct may supply this value.
				reflect.ValueOf(&cfg.WebConfig).Elem().FieldByIndex(field.Index).Set(sentinel)
			}

			data, err := cfg.encode()
			if err != nil {
				t.Fatal(err)
			}
			if !contains(string(data)) {
				t.Errorf("%s: encode() did not carry the value through the copy block:\n%s", key, data)
			}
		})
	}
}

// tomlField finds the field of struct type t tagged with the given top-level
// toml name.
func tomlField(t reflect.Type, name string) (reflect.StructField, bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := strings.SplitN(f.Tag.Get("toml"), ",", 2)[0]
		if tag == name {
			return f, true
		}
	}
	return reflect.StructField{}, false
}

// sentinelFor produces a value of typ that the zero value could not, plus a
// matcher for that value's rendering in encoded TOML. managedKeys today are
// string, *bool and []string; a type outside that set fails loudly rather
// than silently skipping the key it cannot cover.
func sentinelFor(t *testing.T, typ reflect.Type, key string) (reflect.Value, func(string) bool) {
	t.Helper()
	switch typ {
	case reflect.TypeOf(""):
		v := "sentinel-" + key
		return reflect.ValueOf(v), func(s string) bool { return strings.Contains(s, v) }
	case reflect.TypeOf((*bool)(nil)):
		v := true
		return reflect.ValueOf(&v), func(s string) bool { return strings.Contains(s, key+" = true") }
	case reflect.TypeOf([]string(nil)):
		v := []string{"sentinel-" + key}
		return reflect.ValueOf(v), func(s string) bool { return strings.Contains(s, "sentinel-"+key) }
	default:
		t.Fatalf("no sentinel strategy for managed field type %s (key %s)", typ, key)
		return reflect.Value{}, nil
	}
}
