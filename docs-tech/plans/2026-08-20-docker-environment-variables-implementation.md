# Environment variables for the container — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Twelve `EASYWALL_*` variables let a container operator set where easywall
runs, without editing a TOML file first — and no variable can reach a field the
interface writes.

**Architecture:** One new file, `internal/shared/env.go`, holding two typed
tables. Each entry carries its own setter function, so the table *is* the wiring
and cannot drift from it. Both `LoadConfig` functions apply their table just
before returning, which puts the overlay ahead of the `Validate()` that `main`
calls next. `internal/web/config.go` keeps a pre-overlay copy of the parsed file
so the `encode()` fallback can never write an environment value to disk.

**Tech Stack:** Go, `BurntSushi/toml` (already vendored), `reflect` and
`strconv` from the standard library. No new dependencies.

Design document: `docs-tech/plans/2026-08-20-docker-environment-variables.md`.

## Global Constraints

- **No variable may target a field the interface writes.** Excluded: everything
  under `firewall`, `ipv6`, `docker`, `routing`, `acceptance`, plus `telemetry`,
  `username`, `password`, `session_key`, `totp_secret`, `recovery_codes`.
- **No secrets.** Environment variables appear in `docker inspect` and
  `/proc/<pid>/environ`.
- **The overlay runs before `Validate()`**, so an environment value is checked
  exactly like a file value.
- **An environment value must never become file content**, by any path.
- An **empty** variable counts as unset. `-e EASYWALL_WEB_LANGUAGE=` must not
  blank the language.
- An **unparseable boolean is a startup error** naming the variable and the
  value — never a silent fall-back to the default.
- Documentation is part of the change, both the page and the nav entry.
- Go toolchain and every derived pin stay where `go.mod` says; do not touch them.

---

### Task 1: The variable tables and the overlay

**Files:**
- Create: `internal/shared/env.go`
- Test: `internal/shared/env_test.go`

**Interfaces:**
- Consumes: `shared.CoreConfig`, `shared.WebConfig` from `internal/shared/models.go`.
- Produces:
  - `type EnvKind int`, constants `EnvString`, `EnvBool`
  - `type CoreEnvVar struct { Name, TOMLKey string; Kind EnvKind; Set func(*CoreConfig, string) error }`
  - `type WebEnvVar struct { Name, TOMLKey string; Kind EnvKind; Set func(*WebConfig, string) error }`
  - `var CoreEnvVars []CoreEnvVar` (3 entries), `var WebEnvVars []WebEnvVar` (9 entries)
  - `func ApplyCoreEnv(cfg *CoreConfig) error`
  - `func ApplyWebEnv(cfg *WebConfig) error`
  - unexported `applyCoreEnv(cfg *CoreConfig, look func(string) (string, bool)) error` and
    `applyWebEnv(cfg *WebConfig, look func(string) (string, bool)) error` — the
    injectable form the tests drive.

- [ ] **Step 1: Write the failing test**

Create `internal/shared/env_test.go`:

```go
package shared

import "testing"

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
	err := applyCoreEnv(&cfg, lookup(map[string]string{
		"EASYWALL_CORE_SOCKET_PATH": "/run/e.sock",
		"EASYWALL_CORE_DATA_DIR":    "/data",
		"EASYWALL_CORE_LOG_DIR":     "/logs",
	}))
	if err != nil {
		t.Fatalf("applyCoreEnv: %v", err)
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
	err := applyWebEnv(&cfg, lookup(map[string]string{
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
		t.Fatalf("applyWebEnv: %v", err)
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
			if err := applyWebEnv(&cfg, lookup(env)); err != nil {
				t.Fatalf("applyWebEnv: %v", err)
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
	err := applyWebEnv(&cfg, lookup(map[string]string{
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
```

The file's imports are `"strings"` and `"testing"`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/shared/ -run TestApply -v`
Expected: FAIL — `undefined: applyCoreEnv`, `undefined: applyWebEnv`.

- [ ] **Step 3: Write the implementation**

Create `internal/shared/env.go`:

```go
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

// CoreEnvVar binds one variable to the easywall.toml key it overrides.
//
// Set is part of the entry rather than a switch elsewhere on purpose: a table
// beside a switch is two lists to keep in step, and one of them eventually
// stops matching in silence.
type CoreEnvVar struct {
	Name    string
	TOMLKey string
	Kind    EnvKind
	Set     func(*CoreConfig, string) error
}

// WebEnvVar binds one variable to the web.toml key it overrides.
type WebEnvVar struct {
	Name    string
	TOMLKey string
	Kind    EnvKind
	Set     func(*WebConfig, string) error
}

// CoreEnvVars is every variable easywall-core reads.
var CoreEnvVars = []CoreEnvVar{
	{"EASYWALL_CORE_SOCKET_PATH", "socket_path", EnvString,
		func(c *CoreConfig, v string) error { c.SocketPath = v; return nil }},
	{"EASYWALL_CORE_DATA_DIR", "data_dir", EnvString,
		func(c *CoreConfig, v string) error { c.DataDir = v; return nil }},
	{"EASYWALL_CORE_LOG_DIR", "log_dir", EnvString,
		func(c *CoreConfig, v string) error { c.LogDir = v; return nil }},
}

// WebEnvVars is every variable easywall-web reads.
var WebEnvVars = []WebEnvVar{
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
func ApplyCoreEnv(cfg *CoreConfig) error { return applyCoreEnv(cfg, os.LookupEnv) }

// ApplyWebEnv overlays the environment onto a parsed web.toml.
func ApplyWebEnv(cfg *WebConfig) error { return applyWebEnv(cfg, os.LookupEnv) }

func applyCoreEnv(cfg *CoreConfig, look func(string) (string, bool)) error {
	for _, v := range CoreEnvVars {
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

func applyWebEnv(cfg *WebConfig, look func(string) (string, bool)) error {
	for _, v := range WebEnvVars {
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/shared/ -run TestApply -v`
Expected: PASS, four tests.

- [ ] **Step 5: Commit**

```bash
git add internal/shared/env.go internal/shared/env_test.go
git commit -m "feat(config): the environment variable tables and the overlay"
```

---

### Task 2: Both LoadConfig functions apply their table

**Files:**
- Modify: `internal/core/config.go` (`LoadConfig`, around line 66)
- Modify: `internal/web/config.go` (`LoadConfig`, around line 76)
- Test: `internal/core/config_env_test.go` (create), `internal/web/config_env_test.go` (create)

**Interfaces:**
- Consumes: `shared.ApplyCoreEnv`, `shared.ApplyWebEnv` from Task 1.
- Produces: no new exported names. `LoadConfig` keeps its signature
  `func LoadConfig(path string) (*Config, error)` in both packages.

- [ ] **Step 1: Write the failing tests**

Create `internal/core/config_env_test.go`:

```go
package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_EnvOverridesTheFile(t *testing.T) {
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
	if cfg.SocketPath != "/from/env.sock" {
		t.Errorf("SocketPath = %q, want the environment's /from/env.sock", cfg.SocketPath)
	}
}
```

Create `internal/web/config_env_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/core/ ./internal/web/ -run TestLoadConfig_ -v`
Expected: FAIL — the environment is ignored, so `SocketPath` is `/from/file.sock`
and `BindAddr` is `127.0.0.1:1111`.

- [ ] **Step 3: Write the implementation**

In `internal/core/config.go`, replace the tail of `LoadConfig`:

```go
	cfg.configPath = path
	// Before the caller's Validate(), which main runs next: an environment value
	// has to face the same checks a file value does.
	if err := shared.ApplyCoreEnv(&cfg.CoreConfig); err != nil {
		return nil, fmt.Errorf("environment: %w", err)
	}
	return &cfg, nil
```

In `internal/web/config.go`, replace the tail of `LoadConfig`:

```go
	cfg.configPath = path
	if err := shared.ApplyWebEnv(&cfg.WebConfig); err != nil {
		return nil, fmt.Errorf("environment: %w", err)
	}
	return &cfg, nil
```

Add the `shared` import to either file if it is not already there.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/core/ ./internal/web/ -run TestLoadConfig_ -v`
Expected: PASS, four tests.

- [ ] **Step 5: Run the full suite — nothing else may move**

Run: `go test ./internal/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/core/config.go internal/core/config_env_test.go \
        internal/web/config.go internal/web/config_env_test.go
git commit -m "feat(config): read the environment in both LoadConfig paths"
```

---

### Task 3: The overlay never reaches the config file

**Files:**
- Modify: `internal/web/config.go` — the `Config` struct (around line 40), `LoadConfig`, and `encode()` (around line 503)
- Test: `internal/web/config_env_test.go` (append)

**Interfaces:**
- Consumes: `shared.ApplyWebEnv` from Task 1, the `LoadConfig` change from Task 2.
- Produces: an unexported `fileConfig shared.WebConfig` field on `web.Config`.
  No exported surface changes.

**Why:** `saveLocked` renders through `mergeConfig`, which rewrites only
`managedKeys` — none of which is settable from the environment. But `render`
falls back to `encode()` when `mergeConfig` declines (an empty file, or one
stating a key twice), and `encode()` marshals the whole `WebConfig`. On that
path the next password change would write `EASYWALL_WEB_BIND_ADDR`'s value into
`web.toml` permanently.

- [ ] **Step 1: Write the failing test**

Append to `internal/web/config_env_test.go`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/web/ -run TestEnvOverlayNeverReachesTheConfigFile -v`
Expected: FAIL — the file contains `0.0.0.0:31337`.

- [ ] **Step 3: Write the implementation**

In `internal/web/config.go`, add the field to `Config`:

```go
type Config struct {
	shared.WebConfig

	// mu guards WebConfig. Use the accessors rather than the embedded fields.
	mu         sync.RWMutex
	configPath string

	// fileConfig is the parsed file as it stood before the environment overlay.
	// encode() renders this rather than the live struct, so a variable set for
	// the process cannot become content of the operator's file. Only the six
	// managedKeys are taken from the live struct — those are the keys the
	// interface deliberately maintains.
	fileConfig shared.WebConfig
}
```

In `LoadConfig`, capture it before the overlay:

```go
	cfg.configPath = path
	cfg.fileConfig = cfg.WebConfig // before the overlay, deliberately
	if err := shared.ApplyWebEnv(&cfg.WebConfig); err != nil {
		return nil, fmt.Errorf("environment: %w", err)
	}
	return &cfg, nil
```

Replace `encode()`'s body so it renders `fileConfig` with the managed keys taken
from the live struct:

```go
func (c *Config) encode() ([]byte, error) {
	out := c.fileConfig
	// The six keys the interface owns come from the live struct; everything else
	// is what the file said. See the note on fileConfig.
	out.SessionKey = c.WebConfig.SessionKey
	out.Username = c.WebConfig.Username
	out.Password = c.WebConfig.Password
	out.Telemetry = c.WebConfig.Telemetry
	out.TOTPSecret = c.WebConfig.TOTPSecret
	out.RecoveryCodes = c.WebConfig.RecoveryCodes

	buf := bytes.NewBufferString(configHeader)
	if err := toml.NewEncoder(buf).Encode(out); err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	return buf.Bytes(), nil
}
```

Keep the existing `#nosec G117` comment block above the function; it still
applies.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/web/ -run TestEnvOverlayNeverReachesTheConfigFile -v`
Expected: PASS.

- [ ] **Step 5: Run the whole web suite**

Run: `go test ./internal/web/`
Expected: PASS. `SaveCredentials`, `SaveTOTP` and first-run tests exercise
`encode()`; if any fails, the managed-key copy above is missing a field.

- [ ] **Step 6: Commit**

```bash
git add internal/web/config.go internal/web/config_env_test.go
git commit -m "fix(config): keep the environment overlay out of web.toml"
```

---

### Task 4: The guard that keeps the rule true

**Files:**
- Test: `internal/shared/env_guard_test.go` (create)
- Test: `internal/web/env_guard_test.go` (create)

**Interfaces:**
- Consumes: `shared.CoreEnvVars`, `shared.WebEnvVars` (Task 1); `managedKeys`
  (unexported, already in `internal/web/config.go` line 494);
  `tomlKeys(reflect.Type) []string` and `repoFile(t, parts...)`, both already in
  `internal/shared/config_docs_test.go`.
- Produces: no production code.

**Why two tests:** the rule has two halves living in two packages. The rule
fields are types in `internal/shared`; `managedKeys` is unexported in
`internal/web`. Each test sits where it can see its half without widening an
API for a test's benefit.

- [ ] **Step 1: Write the failing tests**

Create `internal/shared/env_guard_test.go`:

```go
package shared

import (
	"reflect"
	"testing"
)

// No environment variable may name a field the interface writes.
//
// Derived, not restated: the forbidden set comes from the payload types the
// interface actually sends — SaveOptions carries FirewallOptions, SaveSettings
// carries NetworkSettings, SaveSystem carries SystemSettings. A test that
// merely listed forbidden keys would be edited in the same commit that added
// the offending variable, which is no protection at all.
//
// What it protects against is concrete: acceptance.duration looks like
// deployment. It is not. If it were settable from the environment, an operator
// would change the acceptance window in the interface, be told it was saved,
// and find the old value back after the next restart.
func TestNoEnvVarTargetsARuleField(t *testing.T) {
	forbidden := map[string]string{} // toml key -> the type it came from
	for _, payload := range []struct {
		name string
		typ  reflect.Type
	}{
		{"FirewallOptions", reflect.TypeOf(FirewallOptions{})},
		{"AcceptanceConfig", reflect.TypeOf(AcceptanceConfig{})},
		{"IPv6Config", reflect.TypeOf(IPv6Config{})},
		{"DockerConfig", reflect.TypeOf(DockerConfig{})},
		{"RoutingConfig", reflect.TypeOf(RoutingConfig{})},
	} {
		for _, k := range tomlKeys(payload.typ) {
			forbidden[k] = payload.name
		}
	}
	if len(forbidden) == 0 {
		t.Fatal("derived no forbidden keys; tomlKeys or the payload types changed shape")
	}

	for _, v := range CoreEnvVars {
		if from, bad := forbidden[v.TOMLKey]; bad {
			t.Errorf("%s targets %q, which the interface writes through %s\n"+
				"  an operator would press Save, be told it was saved, and find the "+
				"old value back after the next restart", v.Name, v.TOMLKey, from)
		}
	}
	for _, v := range WebEnvVars {
		if from, bad := forbidden[v.TOMLKey]; bad {
			t.Errorf("%s targets %q, which the interface writes through %s\n"+
				"  an operator would press Save, be told it was saved, and find the "+
				"old value back after the next restart", v.Name, v.TOMLKey, from)
		}
	}
}

// Every entry names a key that exists, so a typo cannot produce a variable that
// is documented, tested against the rule, and wired to nothing.
func TestEveryEnvVarNamesARealTOMLKey(t *testing.T) {
	core := map[string]bool{}
	for _, k := range tomlKeys(reflect.TypeOf(CoreConfig{})) {
		core[k] = true
	}
	for _, v := range CoreEnvVars {
		if !core[v.TOMLKey] {
			t.Errorf("%s names %q, which is not a key of CoreConfig", v.Name, v.TOMLKey)
		}
	}

	web := map[string]bool{}
	for _, k := range tomlKeys(reflect.TypeOf(WebConfig{})) {
		web[k] = true
	}
	// tls.cert and tls.key are nested one level down.
	for _, k := range tomlKeys(reflect.TypeOf(TLSConfig{})) {
		web["tls."+k] = true
	}
	for _, v := range WebEnvVars {
		if !web[v.TOMLKey] {
			t.Errorf("%s names %q, which is not a key of WebConfig", v.Name, v.TOMLKey)
		}
	}
}
```

Create `internal/web/env_guard_test.go`:

```go
package web

import (
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The other half of the rule: no variable may name one of the six keys the
// interface writes back through mergeConfig. Four of them are secrets, and an
// environment variable is visible in `docker inspect` and /proc/<pid>/environ.
func TestNoEnvVarTargetsAManagedKey(t *testing.T) {
	managed := map[string]bool{}
	for _, k := range managedKeys {
		managed[k] = true
	}
	if len(managed) == 0 {
		t.Fatal("managedKeys is empty; this test would pass for the wrong reason")
	}
	for _, v := range shared.WebEnvVars {
		if managed[v.TOMLKey] {
			t.Errorf("%s targets %q, which the interface writes and saveLocked "+
				"persists — and which is a secret or a consent flag", v.Name, v.TOMLKey)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they pass, then prove they can fail**

Run: `go test ./internal/shared/ ./internal/web/ -run "TestNoEnvVar|TestEveryEnvVarNames" -v`
Expected: PASS.

A guard that has never been seen red is not yet a guard. Temporarily append to
`WebEnvVars` in `internal/shared/env.go`:

```go
	{"EASYWALL_WEB_TELEMETRY", "telemetry", EnvBool,
		func(c *WebConfig, v string) error { b, _ := parseBool(v); c.Telemetry = &b; return nil }},
```

Run the same command.
Expected: FAIL in `TestNoEnvVarTargetsAManagedKey`, naming `telemetry`.

Now change that same temporary entry's `TOMLKey` from `"telemetry"` to
`"duration"` — the acceptance window's key, and the exact case the test's
comment describes:

```go
	{"EASYWALL_WEB_ACCEPTANCE_DURATION", "duration", EnvBool,
		func(c *WebConfig, v string) error { return nil }},
```

Run the same command.
Expected: FAIL in `TestNoEnvVarTargetsARuleField`, naming `duration` and
`AcceptanceConfig`.

**Remove the temporary entry.** Re-run and confirm PASS. Both halves of the rule
have now been seen red; a guard that has only ever been green proves nothing.

- [ ] **Step 3: Commit**

```bash
git add internal/shared/env_guard_test.go internal/web/env_guard_test.go
git commit -m "test(config): no environment variable may target a field the interface writes"
```

---

### Task 5: The documentation page, the nav entry, and the test that ties them

**Files:**
- Create: `docs/_docs/environment.md`
- Modify: `docs/_config.yml` — the `nav:` list, after the Configuration entry (around line 97)
- Modify: `docs/_docs/installation/docker.md` — a pointer to the new page
- Modify: `docs-tech/invariants.md` — the new tests
- Test: `internal/shared/env_docs_test.go` (create)

**Interfaces:**
- Consumes: `shared.CoreEnvVars`, `shared.WebEnvVars` (Task 1); `repoFile(t, parts...)`
  from `internal/shared/config_docs_test.go`.
- Produces: no production code.

- [ ] **Step 1: Write the failing test**

Create `internal/shared/env_docs_test.go`:

```go
package shared

import (
	"regexp"
	"strings"
	"testing"
)

// The page exists so somebody can find the list. A variable missing from it is
// a variable nobody knows about; a variable on it that the code does not read
// is worse, because the reader will set it and believe it took effect.
//
// The same shape as TestEveryConfigKeyIsDocumented, applied to the environment.
func TestEveryEnvVarIsDocumented(t *testing.T) {
	page := repoFile(t, "docs", "_docs", "environment.md")

	var names []string
	for _, v := range CoreEnvVars {
		names = append(names, v.Name)
	}
	for _, v := range WebEnvVars {
		names = append(names, v.Name)
	}

	documented := map[string]bool{}
	for _, m := range regexp.MustCompile(`EASYWALL_[A-Z_]+`).FindAllString(page, -1) {
		documented[m] = true
	}

	for _, n := range names {
		if !documented[n] {
			t.Errorf("%s is read by the code and absent from docs/_docs/environment.md", n)
		}
		delete(documented, n)
	}
	for n := range documented {
		t.Errorf("docs/_docs/environment.md documents %s, which nothing reads", n)
	}
}

// A page with no nav entry is reachable only by its URL, which for a page whose
// whole purpose is being findable is the same as not adding it.
func TestTheEnvironmentPageIsInTheNav(t *testing.T) {
	cfg := repoFile(t, "docs", "_config.yml")
	if !strings.Contains(cfg, "/docs/environment/") {
		t.Error("docs/_config.yml has no nav entry pointing at /docs/environment/")
	}
}

// TZ is not easywall's variable — the Go runtime reads it through tzdata. An
// operator who sets it needs to know it works; a maintainer grepping for it
// needs to know why it is not in the tables.
func TestTheEnvironmentPageExplainsTZ(t *testing.T) {
	page := repoFile(t, "docs", "_docs", "environment.md")
	if !strings.Contains(page, "TZ") {
		t.Error("docs/_docs/environment.md never mentions TZ")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/shared/ -run "TestEveryEnvVarIsDocumented|TestTheEnvironmentPage" -v`
Expected: FAIL — `could not locate docs/_docs/environment.md`.

- [ ] **Step 3: Write the page**

Create `docs/_docs/environment.md`, starting with exactly this front matter —
the same three keys `docs/_docs/configuration.md` carries:

```yaml
---
layout: default
title: Environment Variables
description: Every environment variable easywall reads, and why the list stops where it does.
---
```

The page must contain, in this order:

1. One sentence saying what the environment layer is for: *where easywall runs*,
   not what the firewall does.
2. The `easywall-core` table: `EASYWALL_CORE_SOCKET_PATH`,
   `EASYWALL_CORE_DATA_DIR`, `EASYWALL_CORE_LOG_DIR`, each with its
   `easywall.toml` key and one line of purpose.
3. The `easywall-web` table: `EASYWALL_WEB_BIND_ADDR`,
   `EASYWALL_WEB_SOCKET_PATH`, `EASYWALL_WEB_SSL_DIR`, `EASYWALL_WEB_DATA_DIR`,
   `EASYWALL_WEB_TLS_CERT`, `EASYWALL_WEB_TLS_KEY`, `EASYWALL_WEB_LANGUAGE`,
   `EASYWALL_WEB_UPDATE_CHECK`, `EASYWALL_WEB_DEMO_MODE`, each with its
   `web.toml` key, its type, and one line of purpose.
4. A `TZ` note: read by the Go runtime through tzdata, not by easywall, which is
   why it is not in the tables above. It sets the zone the interface renders
   timestamps in.
5. **What you cannot set here, and why** — the rule in one sentence (*the
   environment configures where easywall runs; the interface configures what the
   firewall does*), then: rule settings and the acceptance window are written by
   the interface, so a variable would be silently undone by the next restart;
   credentials, the session key, the TOTP secret and the recovery codes are
   secrets, and an environment variable is visible in `docker inspect` and
   `/proc/<pid>/environ`.
6. Behaviour notes: an empty variable counts as unset; a boolean that is not
   `true` or `false` stops the process at startup with the variable named.
7. A complete `docker-compose.yml` that runs as written. Base it on the
   repository's own `docker-compose.yml` — keep `TZ`, `network_mode: host` and
   the `NET_ADMIN` capability exactly as that file has them, and add a handful
   of the variables above.

Write it as prose with tables, matching the density of `configuration.md`. Do not
quote a version number anywhere.

- [ ] **Step 4: Add the nav entry**

In `docs/_config.yml`, insert directly after the Configuration entry:

```yaml
      - title: Environment Variables
        path: /docs/environment/
```

- [ ] **Step 5: Point at it from the Docker page**

In `docs/_docs/installation/docker.md`, add one sentence linking to
`/docs/environment/` where the compose file or configuration is first discussed.
Use the same link style the page already uses for internal links — read it first.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/shared/ -run "TestEveryEnvVarIsDocumented|TestTheEnvironmentPage" -v`
Expected: PASS, three tests.

- [ ] **Step 7: Build the site and look at the page**

Run: `cd docs && bundle exec jekyll build`
Expected: builds with no error, and `docs/_site/docs/environment/index.html`
exists.

Then render it and *look* at it — this repository's rule is that a page is
verified by rendering, not by reading its source. Serve `docs/_site` and open
`/docs/environment/` at 1600px and 390px, in both themes. Check that the tables
do not scroll the page sideways at 390px.

- [ ] **Step 8: Record the tests in the invariants document**

In `docs-tech/invariants.md`, add rows for
`TestNoEnvVarTargetsARuleField`, `TestNoEnvVarTargetsAManagedKey`,
`TestEveryEnvVarNamesARealTOMLKey`, `TestEnvOverlayNeverReachesTheConfigFile`,
`TestEveryEnvVarIsDocumented` and `TestTheEnvironmentPageIsInTheNav`, each with
what it protects and the reason it exists — the same two-column shape the file
already uses.

- [ ] **Step 9: Full suite, lint, and commit**

```bash
go test ./internal/...
make lint
git add docs/_docs/environment.md docs/_config.yml \
        docs/_docs/installation/docker.md docs-tech/invariants.md \
        internal/shared/env_docs_test.go
git commit -m "docs: the environment variables a container can set, and why the list ends there"
```

---

### Task 6: The changelog entry

**Files:**
- Modify: `CHANGELOG.md` — under `## [Unreleased]`

- [ ] **Step 1: Add the entry**

Under `## [Unreleased]`, in an `### Added` section (create it if the section is
not there yet), add one entry in the voice the file already uses: what was
impossible before, what is possible now, and — the part that matters — why the
list stops where it does. Name the rule: the environment configures where
easywall runs, the interface configures what the firewall does, and a variable
that crossed that line would make the Options page lie.

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs(changelog): environment variables for the container"
```

---

## Notes for the implementer

- **Do not touch `go.mod`'s `toolchain` line or any Go version pin.** They are
  kept in step by `TestGoToolchainIsTheSameEverywhere` and moved only by
  Renovate. See `docs-tech/dependencies.md`.
- `t.Setenv` makes a test non-parallel by design. Do not add `t.Parallel()` to
  the tests in Tasks 2 and 3.
- If `make lint` fails locally with *"the Go language version used to build
  golangci-lint is lower than the targeted Go version"*, that is a stale local
  binary, not a code fault — CI installs its own. Say so rather than working
  around it.
