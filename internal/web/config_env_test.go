package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeWebConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "web.toml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfig_EnvOverridesTheFile(t *testing.T) {
	path := writeWebConfig(t, "bind_addr = \"127.0.0.1:1111\"\n")
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
