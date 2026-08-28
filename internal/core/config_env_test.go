package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/jp1337/easywall/config"
	"github.com/jp1337/easywall/internal/shared"
)

// A file value the operator wrote beats the variable. This is the 2.12 change:
// until then the environment won, so a value set in the interface came back
// changed after a restart.
func TestLoadConfig_AStoredValueBeatsTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "easywall.toml")
	if err := os.WriteFile(path, []byte("socket_path = \"/from/file.sock\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EASYWALL_CORE_SOCKET_PATH", "/from/env.sock")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.SocketPath != "/from/file.sock" {
		t.Errorf("SocketPath = %q, want the stored /from/file.sock", cfg.SocketPath)
	}
}

// And the variable beats a file that only repeats the built-in default, which is
// every containerised installation.
func TestLoadConfig_EnvBeatsTheShippedDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "easywall.toml")
	if err := os.WriteFile(path, config.Core, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EASYWALL_CORE_SOCKET_PATH", "/from/env.sock")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.SocketPath != "/from/env.sock" {
		t.Errorf("SocketPath = %q, want the environment's /from/env.sock", cfg.SocketPath)
	}
}

// saveLocked encodes the live CoreConfig, and the environment overlay lands in
// that struct. Every save from the Options, Network or System page goes through
// it, so an environment value was being written permanently into the file the
// *root* daemon reads. Measured before the fix, with
// EASYWALL_CORE_DATA_DIR=/data-from-env and a file saying /var/lib/easywall:
// one save rewrote the file to /data-from-env, and remove the variable
// afterwards and the daemon reads rules.json, the apply state and the panic
// marker from a directory the operator never configured.
//
// The web half is guarded by TestEnvOverlayNeverReachesTheConfigFile in
// internal/web; this is the same guard on the half that speaks netlink.
//
// Driven from shared.CoreEnvVars rather than a hand-written list, because
// saveLocked's restore *is* a hand-written list: a variable added to that table
// and not to the restore fails here rather than in an operator's file.
func TestEnvOverlayNeverReachesTheConfigFile(t *testing.T) {
	for _, v := range shared.CoreEnvVars {
		t.Run(v.Name, func(t *testing.T) {
			if v.Kind != shared.EnvString {
				t.Fatalf("%s is %v; this test only knows how to build a string sentinel, "+
					"so it would skip the variable in silence", v.Name, v.Kind)
			}
			sentinel := "/from-the-environment" + filepath.Join("/", v.TOMLKey)
			// The shipped file, not writeCoreConfig's fixture: writeCoreConfig sets
			// socket_path, data_dir and log_dir to values of its own choosing, which
			// the 2.12 precedence rule now reads as stored and would let win over
			// the variable this subtest sets — proving nothing about the leak this
			// test exists to catch. The shipped defaults are also the more honest
			// fixture: this is the file every container image ships.
			path := filepath.Join(t.TempDir(), "easywall.toml")
			if err := os.WriteFile(path, config.Core, 0600); err != nil {
				t.Fatal(err)
			}

			// What the file says for this key, which is what has to survive.
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var onDisk map[string]any
			if err := toml.Unmarshal(original, &onDisk); err != nil {
				t.Fatal(err)
			}
			fromFile, ok := onDisk[v.TOMLKey].(string)
			if !ok || fromFile == "" {
				t.Fatalf("the fixture does not set %s, so this subtest proves nothing", v.TOMLKey)
			}

			t.Setenv(v.Name, sentinel)
			cfg, err := LoadConfig(path)
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			// Assert the overlay applied first: without this the test passes on a
			// variable that never took effect.
			var live bytes.Buffer
			if err := toml.NewEncoder(&live).Encode(cfg.CoreConfig); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(live.String(), sentinel) {
				t.Fatalf("%s did not reach the live configuration, so this subtest proves nothing:\n%s",
					v.Name, live.String())
			}

			// Any Save* takes the same path. System settings is the cheapest.
			if err := cfg.SaveSystemSettings(shared.SystemSettings{
				Acceptance: shared.AcceptanceConfig{Enabled: true, Duration: 300},
			}); err != nil {
				t.Fatalf("SaveSystemSettings: %v", err)
			}

			written, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(written), sentinel) {
				t.Errorf("%s was written into the config file:\n%s", v.Name, written)
			}
			if !strings.Contains(string(written), fromFile) {
				t.Errorf("%s lost the value the file had (%q):\n%s", v.TOMLKey, fromFile, written)
			}
			// The write that was actually asked for still has to have happened.
			if !strings.Contains(string(written), "duration = 300") {
				t.Errorf("the acceptance duration was not persisted:\n%s", written)
			}
		})
	}
}
