package core

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The two states a polling script could not tell apart.
//
// `acceptance: idle` is what the status reports both when no window happens to
// be open and when no window will ever open, and a real host's monitoring
// polled it ten times and reported a true statement about a cause it could not
// see. acceptance_enabled is the field that separates them, so the assertion is
// on both at once: idle *and* not enabled.
func TestFirewallStatus_ReportsWhetherAWindowIsConfiguredAtAll(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Acceptance.Enabled = false
	fw := newTestFirewall(t, cfg)

	st := fw.Status()
	if st.Acceptance != shared.AcceptanceIdle {
		t.Fatalf("Acceptance = %q, want %q — the premise of this test is that the "+
			"status still reads idle with the window switched off",
			st.Acceptance, shared.AcceptanceIdle)
	}
	if st.AcceptanceEnabled {
		t.Error("AcceptanceEnabled is true with acceptance.enabled = false")
	}

	// Over the wire, which is where the script reads it: the json tag is part
	// of the contract docs-tech/protocol.md now names.
	var wire map[string]any
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if got, ok := wire["acceptance_enabled"]; !ok || got != false {
		t.Errorf("acceptance_enabled over the wire = %v (present: %v), want false", got, ok)
	}
}

// The other half of the pair: with the window configured on, the same field
// must say so — otherwise a status that always reported false would pass the
// test above and tell every caller the same lie in the other direction.
func TestFirewallStatus_AcceptanceEnabledFollowsTheConfiguredSwitch(t *testing.T) {
	cfg := newTestConfig(t) // sets Acceptance.Enabled = true
	if !cfg.Acceptance.Enabled {
		t.Fatal("fixture no longer enables the acceptance window; this test asserts nothing")
	}
	if !newTestFirewall(t, cfg).Status().AcceptanceEnabled {
		t.Error("AcceptanceEnabled is false with acceptance.enabled = true")
	}
}

// Config.Validate refuses a duration <= 0 and clamps one out of range, both out
// loud. The switch beside it was silent, so a file naming only `duration` read
// like one that configured a window and configured the length of a window that
// never opened.
func TestValidate_WarnsWhenTheAcceptanceWindowIsSwitchedOff(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Acceptance.Enabled = false    // as an absent [acceptance] enabled key reads
	cfg.IPv6.Mode = shared.IPv6Filter // silence migrateIPv6Mode's own warning
	cfg.Acceptance.Duration = 30      // in range, so the duration warning cannot stand in

	var n int
	prev := slog.Default()
	slog.SetDefault(slog.New(countingHandler{n: &n, substr: "acceptance.enabled"}))
	defer slog.SetDefault(prev)

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate rejected a config with the window off: %v", err)
	}
	if n != 1 {
		t.Errorf("warnings naming acceptance.enabled = %d, want exactly 1", n)
	}
}

func TestValidate_IsSilentWhenTheAcceptanceWindowIsOn(t *testing.T) {
	cfg := newTestConfig(t) // Enabled = true
	cfg.IPv6.Mode = shared.IPv6Filter

	var n int
	prev := slog.Default()
	slog.SetDefault(slog.New(countingHandler{n: &n, substr: "acceptance.enabled"}))
	defer slog.SetDefault(prev)

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if n != 0 {
		t.Errorf("warnings naming acceptance.enabled = %d with the window on, want 0", n)
	}
}

// The first-run warning at every start of a host that has never applied ends
// with a promise: "and it has the acceptance window to undo it". It was gated
// on everConfigured alone and never consulted the switch, so a host with the
// window off was told at every boot that its first apply could be undone.
//
// Anchored on the term, not the sentence: "acceptance window" is the thing
// being promised, and it survives a rewording of the copy around it.
func TestRestoreCurrent_PromisesTheUndoOnlyWhereThereIsOne(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
		want    int
	}{
		{"window on, the clause belongs there", true, 1},
		{"window off, nothing to undo with", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestConfig(t)
			cfg.Acceptance.Enabled = tc.enabled
			// Deliberately *not* configureTestFirewall'd: the branch under test
			// is the one everConfigured guards.
			fw := newTestFirewall(t, cfg)

			var n int
			prev := slog.Default()
			slog.SetDefault(slog.New(countingHandler{n: &n, substr: "acceptance window to undo it"}))
			defer slog.SetDefault(prev)

			if err := fw.RestoreCurrent(RestoreReasonBoot); err != nil {
				t.Fatalf("RestoreCurrent on a never-configured host: %v", err)
			}
			if n != tc.want {
				t.Errorf("first-run warnings promising the acceptance window = %d, want %d", n, tc.want)
			}
		})
	}
}
