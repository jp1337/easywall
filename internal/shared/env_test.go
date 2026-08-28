package shared

import (
	"strings"
	"testing"
)

// lookup builds a fake environment from pairs, so a test never touches the
// real one and can run alongside its neighbours.
func lookup(pairs map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := pairs[k]
		return v, ok
	}
}

func TestApplyCoreEnv_SetsEveryVariable(t *testing.T) {
	var cfg CoreConfig
	_, err := applyEnv(&cfg, CoreConfig{}, CoreEnvVars, lookup(map[string]string{
		"EASYWALL_CORE_SOCKET_PATH": "/run/e.sock",
		"EASYWALL_CORE_DATA_DIR":    "/data",
		"EASYWALL_CORE_LOG_DIR":     "/logs",
	}))
	if err != nil {
		t.Fatalf("applyEnv: %v", err)
	}
	if cfg.SocketPath != "/run/e.sock" {
		t.Errorf("SocketPath = %q, want /run/e.sock", cfg.SocketPath)
	}
	if cfg.DataDir != "/data" {
		t.Errorf("DataDir = %q, want /data", cfg.DataDir)
	}
	if cfg.LogDir != "/logs" {
		t.Errorf("LogDir = %q, want /logs", cfg.LogDir)
	}
}

func TestApplyWebEnv_SetsEveryVariable(t *testing.T) {
	var cfg WebConfig
	_, err := applyEnv(&cfg, WebConfig{}, WebEnvVars, lookup(map[string]string{
		"EASYWALL_WEB_BIND_ADDR":    "0.0.0.0:9999",
		"EASYWALL_WEB_SOCKET_PATH":  "/run/w.sock",
		"EASYWALL_WEB_SSL_DIR":      "/ssl",
		"EASYWALL_WEB_DATA_DIR":     "/wdata",
		"EASYWALL_WEB_TLS_CERT":     "/c.pem",
		"EASYWALL_WEB_TLS_KEY":      "/k.pem",
		"EASYWALL_WEB_LANGUAGE":     "de",
		"EASYWALL_WEB_UPDATE_CHECK": "false",
		"EASYWALL_WEB_DEMO_MODE":    "true",
	}))
	if err != nil {
		t.Fatalf("applyEnv: %v", err)
	}
	if cfg.BindAddr != "0.0.0.0:9999" {
		t.Errorf("BindAddr = %q", cfg.BindAddr)
	}
	if cfg.SocketPath != "/run/w.sock" {
		t.Errorf("SocketPath = %q", cfg.SocketPath)
	}
	if cfg.SSLDir != "/ssl" {
		t.Errorf("SSLDir = %q", cfg.SSLDir)
	}
	if cfg.DataDir != "/wdata" {
		t.Errorf("DataDir = %q", cfg.DataDir)
	}
	if cfg.TLS.CertFile != "/c.pem" {
		t.Errorf("TLS.CertFile = %q", cfg.TLS.CertFile)
	}
	if cfg.TLS.KeyFile != "/k.pem" {
		t.Errorf("TLS.KeyFile = %q", cfg.TLS.KeyFile)
	}
	if cfg.Language != "de" {
		t.Errorf("Language = %q", cfg.Language)
	}
	if cfg.UpdateCheck == nil || *cfg.UpdateCheck {
		t.Errorf("UpdateCheck = %v, want pointer to false", cfg.UpdateCheck)
	}
	if !cfg.DemoMode {
		t.Error("DemoMode = false, want true")
	}
}

// An unset variable leaves the file's value alone; an empty one is unset too,
// so `-e EASYWALL_WEB_LANGUAGE=` cannot blank a language the file set.
func TestApplyWebEnv_UnsetAndEmptyLeaveTheFileValue(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"unset": {},
		"empty": {"EASYWALL_WEB_LANGUAGE": ""},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := WebConfig{Language: "de"}
			if _, err := applyEnv(&cfg, WebConfig{}, WebEnvVars, lookup(env)); err != nil {
				t.Fatalf("applyEnv: %v", err)
			}
			if cfg.Language != "de" {
				t.Errorf("Language = %q, want the file's de", cfg.Language)
			}
		})
	}
}

// A boolean nobody can parse stops the process. Falling back to the default
// would let EASYWALL_WEB_UPDATE_CHECK=yes contact GitHub while the operator
// believes they switched it off.
func TestApplyWebEnv_UnparseableBoolNamesTheVariableAndTheValue(t *testing.T) {
	var cfg WebConfig
	_, err := applyEnv(&cfg, WebConfig{}, WebEnvVars, lookup(map[string]string{
		"EASYWALL_WEB_UPDATE_CHECK": "yes",
	}))
	if err == nil {
		t.Fatal("want an error for EASYWALL_WEB_UPDATE_CHECK=yes, got nil")
	}
	for _, want := range []string{"EASYWALL_WEB_UPDATE_CHECK", "yes"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
