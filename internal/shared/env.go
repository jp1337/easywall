package shared

import (
	"fmt"
	"os"
	"strconv"

	"github.com/BurntSushi/toml"
	"github.com/jp1337/easywall/config"
)

// The environment layer configures *where easywall runs*, and one thing about
// what it reports: whether this installation is counted. It deliberately cannot
// reach what the firewall does — every rule field is absent from the tables
// below, and TestNoEnvVarTargetsARuleField keeps it that way.
//
// telemetry is the one managed key a variable may name, because the public demo
// is configured entirely from its environment and has to report like anything
// else. It is safe to name only since 2.12 inverted the precedence: an operator
// who answers in the interface has stored a value, and a stored value wins. The
// reason the rule existed — press Save, be told it was saved, find the old value
// back after a restart — is the thing that release removed.
//
// Secrets stay absent for a second reason, and that one has not changed: an
// environment variable is visible in `docker inspect`, in /proc/<pid>/environ,
// and in any log somebody pastes into an issue. web.toml is 0600.
// TestNoEnvVarTargetsAManagedKey is what keeps the other five out.

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
//
// Get renders the key's current value the way the operator would write it in
// TOML. It answers the question the precedence rule turns on — has anybody
// stored this key, or does the file merely repeat the built-in default — and it
// is what the interface shows beside a control.
// TestEveryEnvVarRoundTripsThroughGetAndSet keeps the pair describing one field.
type EnvVar[T any] struct {
	Name    string
	TOMLKey string
	Kind    EnvKind
	Get     func(*T) string
	Set     func(*T, string) error
}

// Provenance is where the value in force for one key came from. One exists for
// each variable that is set, and for no other key: a control with no variable
// behind it has nothing to say and draws nothing.
type Provenance struct {
	// Name is the variable, so the marker can name it.
	Name string
	// Env is that variable's value.
	Env string
	// Stored is the file's value when the operator set it — that is, when it
	// differs from the built-in default. Empty both when the file only repeats
	// the default and when an operator deliberately stored "" for a key whose
	// default is also "" — the two are indistinguishable from this value alone,
	// and Env is what is in force either way.
	Stored string
}

// Overridden reports whether a stored value is beating the environment in a way
// worth showing. A stored value identical to the variable's is not a conflict,
// and drawing one would invite an operator to "fix" two values that agree.
func (p Provenance) Overridden() bool { return p.Stored != "" && p.Stored != p.Env }

// boolValue renders a *bool the way TOML would. A nil pointer is the key being
// absent, which is a state config/web.toml uses for telemetry: never asked is
// not the same answer as no.
//
// DemoMode is the one plain (non-pointer) bool below, and its Get skips this
// function entirely — safe only because its shipped default is false, so an
// omitted key decodes to the zero value, which happens to equal the default.
// A plain bool shipping true would decode an omitted key to false and have
// that read as stored, permanently disabling its variable; a *bool is what
// keeps "never set" distinguishable from "set to the default".
func boolValue(b *bool) string {
	if b == nil {
		return ""
	}
	return strconv.FormatBool(*b)
}

// CoreEnvVars is every variable easywall-core reads.
var CoreEnvVars = []EnvVar[CoreConfig]{
	{"EASYWALL_CORE_SOCKET_PATH", "socket_path", EnvString,
		func(c *CoreConfig) string { return c.SocketPath },
		func(c *CoreConfig, v string) error { c.SocketPath = v; return nil }},
	{"EASYWALL_CORE_DATA_DIR", "data_dir", EnvString,
		func(c *CoreConfig) string { return c.DataDir },
		func(c *CoreConfig, v string) error { c.DataDir = v; return nil }},
	{"EASYWALL_CORE_LOG_DIR", "log_dir", EnvString,
		func(c *CoreConfig) string { return c.LogDir },
		func(c *CoreConfig, v string) error { c.LogDir = v; return nil }},
}

// WebEnvVars is every variable easywall-web reads.
var WebEnvVars = []EnvVar[WebConfig]{
	{"EASYWALL_WEB_BIND_ADDR", "bind_addr", EnvString,
		func(c *WebConfig) string { return c.BindAddr },
		func(c *WebConfig, v string) error { c.BindAddr = v; return nil }},
	{"EASYWALL_WEB_SOCKET_PATH", "socket_path", EnvString,
		func(c *WebConfig) string { return c.SocketPath },
		func(c *WebConfig, v string) error { c.SocketPath = v; return nil }},
	{"EASYWALL_WEB_SSL_DIR", "ssl_dir", EnvString,
		func(c *WebConfig) string { return c.SSLDir },
		func(c *WebConfig, v string) error { c.SSLDir = v; return nil }},
	{"EASYWALL_WEB_DATA_DIR", "data_dir", EnvString,
		func(c *WebConfig) string { return c.DataDir },
		func(c *WebConfig, v string) error { c.DataDir = v; return nil }},
	{"EASYWALL_WEB_TLS_CERT", "tls.cert", EnvString,
		func(c *WebConfig) string { return c.TLS.CertFile },
		func(c *WebConfig, v string) error { c.TLS.CertFile = v; return nil }},
	{"EASYWALL_WEB_TLS_KEY", "tls.key", EnvString,
		func(c *WebConfig) string { return c.TLS.KeyFile },
		func(c *WebConfig, v string) error { c.TLS.KeyFile = v; return nil }},
	{"EASYWALL_WEB_LANGUAGE", "language", EnvString,
		func(c *WebConfig) string { return c.Language },
		func(c *WebConfig, v string) error { c.Language = v; return nil }},
	{"EASYWALL_WEB_UPDATE_CHECK", "update_check", EnvBool,
		func(c *WebConfig) string { return boolValue(c.UpdateCheck) },
		func(c *WebConfig, v string) error {
			b, err := parseBool(v)
			if err != nil {
				return err
			}
			c.UpdateCheck = &b
			return nil
		}},
	{"EASYWALL_WEB_DEMO_MODE", "demo_mode", EnvBool,
		func(c *WebConfig) string { return strconv.FormatBool(c.DemoMode) },
		func(c *WebConfig, v string) error {
			b, err := parseBool(v)
			if err != nil {
				return err
			}
			c.DemoMode = b
			return nil
		}},
	{"EASYWALL_WEB_TELEMETRY", "telemetry", EnvBool,
		func(c *WebConfig) string { return boolValue(c.Telemetry) },
		func(c *WebConfig, v string) error {
			b, err := parseBool(v)
			if err != nil {
				return err
			}
			c.Telemetry = &b
			return nil
		}},
}

// WebEnvVar returns the entry for a TOML key. The interface reaches the table
// this way rather than keeping a second copy of it beside the controls.
func WebEnvVar(tomlKey string) (EnvVar[WebConfig], bool) {
	for _, v := range WebEnvVars {
		if v.TOMLKey == tomlKey {
			return v, true
		}
	}
	return EnvVar[WebConfig]{}, false
}

// The built-in defaults are the commented files the package installs — the same
// bytes -write-config produces, embedded once in config/embed.go. Parsed rather
// than restated, because a second list of defaults is a second thing to keep in
// step, and the whole precedence rule is a comparison against this value.
//
// A parse failure here is a broken build, not a runtime condition: these are
// compiled-in bytes, and internal/shared/conffiles_test.go already reads them.
var webDefault, coreDefault = func() (WebConfig, CoreConfig) {
	var w WebConfig
	if _, err := toml.Decode(string(config.Web), &w); err != nil {
		panic("config/web.toml does not parse: " + err.Error())
	}
	var c CoreConfig
	if _, err := toml.Decode(string(config.Core), &c); err != nil {
		panic("config/easywall.toml does not parse: " + err.Error())
	}
	return w, c
}()

// WebDefault returns the built-in easywall-web configuration.
//
// A real copy, not just a struct assignment: UpdateCheck and Telemetry are
// *bool, and RecoveryCodes is a slice, so a plain `return webDefault` would
// still alias the package-level value through those three fields. A caller
// doing *WebDefault().UpdateCheck = false would then reach back and silently
// change what every later comparison in the process compares against — after
// which a file saying update_check = true reads as stored and
// EASYWALL_WEB_UPDATE_CHECK is dead for the rest of the process's life. Cloned
// here so a caller can hold and edit its own copy freely.
func WebDefault() WebConfig {
	d := webDefault
	if d.UpdateCheck != nil {
		v := *d.UpdateCheck
		d.UpdateCheck = &v
	}
	if d.Telemetry != nil {
		v := *d.Telemetry
		d.Telemetry = &v
	}
	d.RecoveryCodes = append([]string(nil), d.RecoveryCodes...)
	return d
}

// CoreDefault returns the built-in easywall-core configuration, cloned for the
// same reason as WebDefault: Docker.CustomNetworks and Routing.Networks are
// slices that a plain struct copy would still alias to the package-level
// value.
func CoreDefault() CoreConfig {
	d := coreDefault
	d.Docker.CustomNetworks = append([]string(nil), d.Docker.CustomNetworks...)
	d.Routing.Networks = append([]string(nil), d.Routing.Networks...)
	return d
}

// ApplyCoreEnv resolves the environment against a parsed easywall.toml.
func ApplyCoreEnv(cfg *CoreConfig) (map[string]Provenance, error) {
	return applyEnv(cfg, CoreDefault(), CoreEnvVars, os.LookupEnv)
}

// ApplyWebEnv resolves the environment against a parsed web.toml.
func ApplyWebEnv(cfg *WebConfig) (map[string]Provenance, error) {
	return applyEnv(cfg, WebDefault(), WebEnvVars, os.LookupEnv)
}

// applyEnv walks one table and resolves each key.
//
// The order is stored value, then environment variable, then built-in default —
// Vaultwarden's, and the inverse of what easywall did until 2.12. The old order
// meant an operator could change a setting in the interface, see it apply, and
// find it reverted after a restart, with nothing anywhere saying why.
//
// "Stored" is the file value differing from def, never the key being present in
// the file: -write-config emits every default, and that file is what the
// container image ships, so presence would disable every variable in the product
// on exactly the installations that use them.
// TestAShippedDefaultDoesNotCountAsStored is what keeps that true.
//
// look is injected so a test never touches the real environment and can run
// beside its neighbours.
func applyEnv[T any](cfg *T, def T, vars []EnvVar[T], look func(string) (string, bool)) (map[string]Provenance, error) {
	prov := make(map[string]Provenance)
	for _, v := range vars {
		raw, ok := present(look, v.Name)
		if !ok {
			continue
		}

		// Checked before anything decides whether it wins. A variable nobody can
		// parse is a mistake worth reporting whether or not a stored value beats
		// it — otherwise the operator who removes the stored value discovers the
		// typo then, on a restart, with the process refusing to start.
		//
		// Canonicalised in place once parsed: `1`, `TRUE` and `t` are the same
		// boolean as a stored `true`, and Provenance.Env below is compared
		// against Stored as plain strings (Provenance.Overridden). Comparing two
		// spellings of one value would show a conflict where none exists —
		// exactly the false "overridden here" the interface must not draw.
		// Reassigning raw also means v.Set below receives the canonical form,
		// and the marker shown to the operator is a well-formed boolean instead
		// of whatever spelling they happened to type.
		if v.Kind == EnvBool {
			b, err := parseBool(raw)
			if err != nil {
				return nil, fmt.Errorf("%s=%q: %w", v.Name, raw, err)
			}
			raw = strconv.FormatBool(b)
		}

		stored := v.Get(cfg)
		if stored == v.Get(&def) {
			stored = "" // the file only repeats the default; nobody set this
		}
		prov[v.TOMLKey] = Provenance{Name: v.Name, Env: raw, Stored: stored}
		if stored != "" {
			continue
		}
		if err := v.Set(cfg, raw); err != nil {
			return nil, fmt.Errorf("%s=%q: %w", v.Name, raw, err)
		}
	}
	return prov, nil
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
