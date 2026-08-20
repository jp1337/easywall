package shared

import (
	"fmt"
	"os"
	"strconv"
)

// The environment layer configures *where easywall runs*. It deliberately
// cannot reach what the firewall does: every rule field, and every setting the
// interface writes back, is absent from the tables below. A variable that
// overrode one of those would let an operator press Save, be told it was saved,
// and find the old value back after the next restart —
// TestNoEnvVarTargetsARuleField and TestNoEnvVarTargetsAManagedKey are what
// keep that true as the config grows.
//
// Secrets are absent for a second reason: an environment variable is visible in
// `docker inspect`, in /proc/<pid>/environ, and in any log somebody pastes into
// an issue. web.toml is 0600.

// EnvKind is the value shape a variable carries.
type EnvKind int

const (
	// EnvString is taken verbatim.
	EnvString EnvKind = iota
	// EnvBool is parsed with strconv.ParseBool; anything else stops startup.
	EnvBool
)

// EnvVar binds one variable to the TOML key it overrides, in a configuration of
// type T.
//
// Set is part of the entry rather than a switch elsewhere on purpose: a table
// beside a switch is two lists to keep in step, and one of them eventually
// stops matching in silence. The type parameter is there for the same reason —
// core and web differ only in their table and their config type, and two
// near-identical loops would be a third thing to keep in step by hand.
type EnvVar[T any] struct {
	Name    string
	TOMLKey string
	Kind    EnvKind
	Set     func(*T, string) error
}

// CoreEnvVars is every variable easywall-core reads.
var CoreEnvVars = []EnvVar[CoreConfig]{
	{"EASYWALL_CORE_SOCKET_PATH", "socket_path", EnvString,
		func(c *CoreConfig, v string) error { c.SocketPath = v; return nil }},
	{"EASYWALL_CORE_DATA_DIR", "data_dir", EnvString,
		func(c *CoreConfig, v string) error { c.DataDir = v; return nil }},
	{"EASYWALL_CORE_LOG_DIR", "log_dir", EnvString,
		func(c *CoreConfig, v string) error { c.LogDir = v; return nil }},
}

// WebEnvVars is every variable easywall-web reads.
var WebEnvVars = []EnvVar[WebConfig]{
	{"EASYWALL_WEB_BIND_ADDR", "bind_addr", EnvString,
		func(c *WebConfig, v string) error { c.BindAddr = v; return nil }},
	{"EASYWALL_WEB_SOCKET_PATH", "socket_path", EnvString,
		func(c *WebConfig, v string) error { c.SocketPath = v; return nil }},
	{"EASYWALL_WEB_SSL_DIR", "ssl_dir", EnvString,
		func(c *WebConfig, v string) error { c.SSLDir = v; return nil }},
	{"EASYWALL_WEB_DATA_DIR", "data_dir", EnvString,
		func(c *WebConfig, v string) error { c.DataDir = v; return nil }},
	{"EASYWALL_WEB_TLS_CERT", "tls.cert", EnvString,
		func(c *WebConfig, v string) error { c.TLS.CertFile = v; return nil }},
	{"EASYWALL_WEB_TLS_KEY", "tls.key", EnvString,
		func(c *WebConfig, v string) error { c.TLS.KeyFile = v; return nil }},
	{"EASYWALL_WEB_LANGUAGE", "language", EnvString,
		func(c *WebConfig, v string) error { c.Language = v; return nil }},
	{"EASYWALL_WEB_UPDATE_CHECK", "update_check", EnvBool,
		func(c *WebConfig, v string) error {
			b, err := parseBool(v)
			if err != nil {
				return err
			}
			c.UpdateCheck = &b
			return nil
		}},
	{"EASYWALL_WEB_DEMO_MODE", "demo_mode", EnvBool,
		func(c *WebConfig, v string) error {
			b, err := parseBool(v)
			if err != nil {
				return err
			}
			c.DemoMode = b
			return nil
		}},
}

// ApplyCoreEnv overlays the environment onto a parsed easywall.toml.
func ApplyCoreEnv(cfg *CoreConfig) error { return applyEnv(cfg, CoreEnvVars, os.LookupEnv) }

// ApplyWebEnv overlays the environment onto a parsed web.toml.
func ApplyWebEnv(cfg *WebConfig) error { return applyEnv(cfg, WebEnvVars, os.LookupEnv) }

// applyEnv walks one table. look is injected so a test never touches the real
// environment and can run beside its neighbours.
func applyEnv[T any](cfg *T, vars []EnvVar[T], look func(string) (string, bool)) error {
	for _, v := range vars {
		raw, ok := present(look, v.Name)
		if !ok {
			continue
		}
		if err := v.Set(cfg, raw); err != nil {
			return fmt.Errorf("%s=%q: %w", v.Name, raw, err)
		}
	}
	return nil
}

// present reports a variable as set only when it carries a value. An empty
// variable is treated as absent, so `-e EASYWALL_WEB_LANGUAGE=` leaves the
// file's language alone rather than blanking it.
func present(look func(string) (string, bool), name string) (string, bool) {
	raw, ok := look(name)
	if !ok || raw == "" {
		return "", false
	}
	return raw, true
}

func parseBool(v string) (bool, error) {
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("not a boolean; use true or false")
	}
	return b, nil
}
