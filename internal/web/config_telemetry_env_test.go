package web

import (
	"os"
	"strings"
	"testing"

	"github.com/jp1337/easywall/config"
)

// The public demo reports, and it is configured entirely from the environment.
func TestTelemetryCanBeSwitchedOnFromTheEnvironment(t *testing.T) {
	path := writeWebConfig(t, string(config.Web))
	t.Setenv("EASYWALL_WEB_TELEMETRY", "true")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.TelemetryEnabled() {
		t.Error("EASYWALL_WEB_TELEMETRY=true did not switch counting on")
	}
}

// Consent is an answer, not a default. An operator who said no in the interface
// has stored a value, and the variable does not get to undo it.
func TestAStoredNoBeatsTelemetryFromTheEnvironment(t *testing.T) {
	// Inserted before [tls] rather than appended after it: config.Web ends with
	// that table, and a bare key appended after a table header belongs to the
	// table, not to the document root — an appended "telemetry = false" would
	// silently become the unrecognised tls.telemetry and never reach the field
	// this test means to set.
	body := strings.Replace(string(config.Web), "[tls]", "telemetry = false\n\n[tls]", 1)
	path := writeWebConfig(t, body)
	t.Setenv("EASYWALL_WEB_TELEMETRY", "true")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.TelemetryEnabled() {
		t.Error("the environment switched counting on over a stored no")
	}
	p, ok := cfg.Provenance("telemetry")
	if !ok {
		t.Fatal("no provenance for telemetry, and the variable is set")
	}
	if !p.Overridden() {
		t.Errorf("provenance = %+v, want the stored value reported as overriding", p)
	}
}

// The variable's value must never become content of the operator's file. It
// would stop being a deployment setting and become a stored one — which then
// beats the very variable it came from, permanently, from the next password
// change onwards.
//
// mergeConfig is the path this takes: it is handed the live configuration, and
// after the overlay the live telemetry value is the environment's.
// TestEnvOverlayNeverReachesTheConfigFile covers the encode() fallback; this
// covers the merge path, which is the one a real file takes.
func TestTelemetryFromTheEnvironmentIsNeverWrittenToTheFile(t *testing.T) {
	path := writeWebConfig(t, string(config.Web))
	t.Setenv("EASYWALL_WEB_TELEMETRY", "true")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.TelemetryEnabled() {
		t.Fatal("the overlay did not apply, so this test proves nothing")
	}

	// Any save takes the render path. A credential change is the realistic one.
	if err := cfg.SaveCredentials("admin", "$argon2id$fake"); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(written), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "telemetry") {
			t.Errorf("the environment's value was written to disk as %q", trimmed)
		}
	}
	if !strings.Contains(string(written), "admin") {
		t.Errorf("the write that was asked for did not happen:\n%s", written)
	}
}

// And the operator's own answer still reaches the file.
func TestSaveTelemetryStillWritesTheOperatorsAnswer(t *testing.T) {
	path := writeWebConfig(t, string(config.Web))
	t.Setenv("EASYWALL_WEB_TELEMETRY", "true")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.SaveTelemetry(false); err != nil {
		t.Fatalf("SaveTelemetry: %v", err)
	}
	if cfg.TelemetryEnabled() {
		t.Error("TelemetryEnabled still reports yes after the operator said no")
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(written2Lines(string(written)), "telemetry = false") {
		t.Errorf("the answer is not in the file:\n%s", written)
	}

	// And it survives a reload against the same environment.
	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if reloaded.TelemetryEnabled() {
		t.Error("the stored no did not survive a restart with the variable still set")
	}
}

// written2Lines collapses whitespace inside assignments so the assertion above
// does not depend on the file's alignment.
func written2Lines(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		out = append(out, strings.Join(fields, " "))
	}
	return strings.Join(out, "\n")
}
