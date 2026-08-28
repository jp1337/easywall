package web

import (
	"os"
	"strings"
	"testing"

	"github.com/jp1337/easywall/config"
)

// "Reset to the environment value" means the file stops stating the key. For
// telemetry it cannot mean "write the default": the default is the key's
// absence, because absent and false are different answers.
func TestResetTelemetryRemovesTheLineAndHandsTheKeyBackToTheEnvironment(t *testing.T) {
	// Inserted before [tls] rather than appended after config.Web: that file
	// ends with the [tls] table, and a bare key appended past a table header
	// decodes into the table, not the document root — this would silently
	// become tls.telemetry and never reach the field this test means to set.
	body := strings.Replace(string(config.Web), "[tls]", "telemetry = false  # my note\n\n[tls]", 1)
	path := writeWebConfig(t, body)
	t.Setenv("EASYWALL_WEB_TELEMETRY", "true")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.TelemetryEnabled() {
		t.Fatal("the stored no did not win, so this test starts from the wrong state")
	}

	if err := cfg.ResetTelemetry(); err != nil {
		t.Fatalf("ResetTelemetry: %v", err)
	}
	if !cfg.TelemetryEnabled() {
		t.Error("after the reset the environment's true is not in force")
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(written), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "telemetry") {
			t.Errorf("the line survived the reset: %q", trimmed)
		}
	}
	// Everything else the file said is still there.
	if !strings.Contains(string(written), "bind_addr") {
		t.Errorf("the reset rewrote more than the one line:\n%s", written)
	}
	// And it took the in-place edit, not the full-encode fallback: a comment
	// from the shipped file survives, and encode()'s "Rebuilt by easywall"
	// banner is absent. Without this, a mutation that broke mergeConfig's new
	// removal branch would still pass every check above — encode() also drops
	// a nil telemetry, just by throwing away every other comment too.
	if !strings.Contains(string(written), "# Set to true to run easywall-web against an in-memory mock") {
		t.Errorf("a comment from the shipped file was lost; the reset fell back to a full re-encode:\n%s", written)
	}
	if strings.Contains(string(written), "Rebuilt by easywall") {
		t.Error("the reset went through the full-encode fallback instead of editing the file in place")
	}

	// And a restart agrees.
	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !reloaded.TelemetryEnabled() {
		t.Error("the reset did not survive a restart")
	}
	if p, ok := reloaded.Provenance("telemetry"); !ok || p.Overridden() {
		t.Errorf("provenance = %+v (present=%v), want the environment in force", p, ok)
	}
}

// A reset with no variable set leaves the operator with the built-in default and
// no stored answer — which for telemetry is "never asked".
func TestResetTelemetryWithNoVariableSetClearsTheAnswer(t *testing.T) {
	// Same [tls]-placement care as above, so this genuinely starts from a
	// stored "yes" rather than one silently swallowed into the wrong table.
	body := strings.Replace(string(config.Web), "[tls]", "telemetry = true\n\n[tls]", 1)
	path := writeWebConfig(t, body)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.TelemetryEnabled() {
		t.Fatal("the stored yes did not take effect, so this test starts from the wrong state")
	}
	if err := cfg.ResetTelemetry(); err != nil {
		t.Fatalf("ResetTelemetry: %v", err)
	}
	if cfg.TelemetryEnabled() {
		t.Error("counting is still on after the answer was cleared")
	}
}
