package web

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// ── Construction & client wiring ─────────────────────────────────────────

func TestNewDemoClient_IsDemo(t *testing.T) {
	c := NewDemoClient()
	if !c.IsDemo() {
		t.Error("NewDemoClient should produce a client where IsDemo() is true")
	}
}

func TestNewCoreClient_NotDemo(t *testing.T) {
	c := NewCoreClient("/some/path")
	if c.IsDemo() {
		t.Error("NewCoreClient should produce a client where IsDemo() is false")
	}
}

// ── Seed values ───────────────────────────────────────────────────────────

func TestDemoState_SeedsRules(t *testing.T) {
	d := newDemoState()
	if len(d.rules.Current.TCP) == 0 {
		t.Error("seed should include at least one TCP port rule")
	}
	if len(d.rules.Current.Whitelist) == 0 {
		t.Error("seed should include at least one whitelist entry")
	}
	if len(d.auditLog) == 0 {
		t.Error("seed should include audit log entries")
	}
}

// ── CmdGetStatus / CmdGetRules ───────────────────────────────────────────

func TestDemoSend_GetStatus(t *testing.T) {
	c := NewDemoClient()
	st, err := c.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if !st.Active {
		t.Error("expected Active=true in seeded state")
	}
	if st.Acceptance != shared.AcceptanceIdle {
		t.Errorf("expected idle acceptance, got %s", st.Acceptance)
	}
}

func TestDemoSend_GetRules(t *testing.T) {
	c := NewDemoClient()
	state, err := c.GetRules()
	if err != nil {
		t.Fatalf("GetRules: %v", err)
	}
	if len(state.Current.TCP) == 0 {
		t.Error("expected seeded TCP rules in Current")
	}
}

// ── CmdSaveRules ─────────────────────────────────────────────────────────

func TestDemoSend_SaveRulesTCP(t *testing.T) {
	c := NewDemoClient()
	newRules := []shared.PortRule{
		{Port: "8080", Description: "alt-http"},
	}
	if err := c.SaveRules("tcp", newRules); err != nil {
		t.Fatalf("SaveRules: %v", err)
	}
	state, _ := c.GetRules()
	if len(state.Staged.TCP) != 1 || state.Staged.TCP[0].Port != "8080" {
		t.Errorf("Staged.TCP should contain 8080 rule, got %+v", state.Staged.TCP)
	}
	// Status should now report HasPending=true since Staged != Current.
	st, _ := c.GetStatus()
	if !st.HasPending {
		t.Error("expected HasPending=true after Save")
	}
}

func TestDemoSend_SaveRulesBlacklist(t *testing.T) {
	c := NewDemoClient()
	if err := c.SaveRules("blacklist", []string{"10.0.0.1", "10.0.0.2"}); err != nil {
		t.Fatalf("SaveRules: %v", err)
	}
	state, _ := c.GetRules()
	if len(state.Staged.Blacklist) != 2 {
		t.Errorf("expected 2 blacklist entries, got %v", state.Staged.Blacklist)
	}
}

func TestDemoSend_SaveRules_UnknownType(t *testing.T) {
	c := NewDemoClient()
	err := c.SaveRules("nonsense", []string{})
	if err == nil {
		t.Error("expected error for unknown rule type")
	}
}

// ── Apply → Accept happy path ────────────────────────────────────────────

func TestDemoSend_ApplyAccept(t *testing.T) {
	c := NewDemoClient()
	// Stage a change so apply is meaningful
	_ = c.SaveRules("tcp", []shared.PortRule{{Port: "9000"}})

	if err := c.ApplyRules(); err != nil {
		t.Fatalf("ApplyRules: %v", err)
	}
	st, _ := c.GetStatus()
	if st.Acceptance != shared.AcceptancePending {
		t.Errorf("expected pending after Apply, got %s", st.Acceptance)
	}

	if accepted, err := c.Accept(); err != nil || !accepted {
		t.Fatalf("Accept: accepted=%v err=%v", accepted, err)
	}
	st, _ = c.GetStatus()
	if st.Acceptance != shared.AcceptanceAccepted {
		t.Errorf("expected accepted after Accept, got %s", st.Acceptance)
	}

	// Current should now match the previously-staged rules
	state, _ := c.GetRules()
	if len(state.Current.TCP) != 1 || state.Current.TCP[0].Port != "9000" {
		t.Errorf("Current.TCP should contain 9000 after Apply+Accept, got %+v", state.Current.TCP)
	}
}

// ── Apply → Rollback (timer fires) ───────────────────────────────────────

func TestDemoSend_ApplyRollback(t *testing.T) {
	c := NewDemoClient()
	// A one-second window fires fast, and the demo now refuses to store one —
	// the permitted range starts at ten seconds, because anything shorter closes
	// before the confirmation page can be read. Set the field directly: this
	// test is about the timer, not about what the settings API accepts.
	c.demo.mu.Lock()
	c.demo.system.Acceptance = shared.AcceptanceConfig{Enabled: true, Duration: 1}
	c.demo.mu.Unlock()

	// Snapshot the originally seeded rules before mutating.
	originalState, _ := c.GetRules()
	originalTCP := originalState.Current.TCP

	// Stage + apply something different.
	_ = c.SaveRules("tcp", []shared.PortRule{{Port: "9999"}})
	if err := c.ApplyRules(); err != nil {
		t.Fatalf("ApplyRules: %v", err)
	}

	// Wait for the timer (1s) + a small buffer.
	time.Sleep(1500 * time.Millisecond)

	st, _ := c.GetStatus()
	if st.Acceptance != shared.AcceptanceRolledBack {
		t.Errorf("expected rolled_back after timer, got %s", st.Acceptance)
	}
	state, _ := c.GetRules()
	if len(state.Current.TCP) != len(originalTCP) {
		t.Errorf("Current should be restored to backup after rollback, got %+v", state.Current.TCP)
	}
}

// ── Apply with acceptance disabled (no timer) ────────────────────────────

func TestDemoSend_ApplyWithoutAcceptance(t *testing.T) {
	c := NewDemoClient()
	// Disable the acceptance window
	payload, _ := json.Marshal(shared.SystemSettings{
		Acceptance: shared.AcceptanceConfig{Enabled: false, Duration: 60},
	})
	resp, _ := c.Send(shared.Command{Type: shared.CmdSaveSystem, Payload: payload})
	if !resp.Success {
		t.Fatalf("SaveSystem: %s", resp.Error)
	}

	if err := c.ApplyRules(); err != nil {
		t.Fatalf("ApplyRules: %v", err)
	}
	// Should jump straight to Accepted with no timer.
	st, _ := c.GetStatus()
	if st.Acceptance != shared.AcceptanceAccepted {
		t.Errorf("expected accepted without acceptance window, got %s", st.Acceptance)
	}
}

// ── Options / Settings / System ──────────────────────────────────────────

func TestDemoSend_OptionsRoundTrip(t *testing.T) {
	c := NewDemoClient()

	original, err := c.GetOptions()
	if err != nil {
		t.Fatalf("GetOptions: %v", err)
	}

	// Flip a value and save.
	mod := *original
	mod.SSHBruteForce = !mod.SSHBruteForce
	if err := c.SaveOptions(mod); err != nil {
		t.Fatalf("SaveOptions: %v", err)
	}

	roundtrip, _ := c.GetOptions()
	if roundtrip.SSHBruteForce == original.SSHBruteForce {
		t.Error("SSHBruteForce should have flipped after Save")
	}
}

func TestDemoSend_SettingsRoundTrip(t *testing.T) {
	c := NewDemoClient()

	mod := shared.NetworkSettings{
		IPv6:   shared.IPv6Config{Enabled: false},
		Docker: shared.DockerConfig{Enabled: true},
	}
	if err := c.SaveSettings(mod); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	roundtrip, _ := c.GetSettings()
	if roundtrip.IPv6.Enabled || !roundtrip.Docker.Enabled {
		t.Errorf("settings did not persist, got %+v", roundtrip)
	}
}

func TestDemoSend_SystemInvalidDuration(t *testing.T) {
	c := NewDemoClient()
	bad := shared.SystemSettings{
		Acceptance: shared.AcceptanceConfig{Enabled: true, Duration: 0},
	}
	err := c.SaveSystem(bad)
	if err == nil {
		t.Error("expected error for zero duration")
	}
}

// ── Validate (custom rules) ──────────────────────────────────────────────

// The demo has no nft binary, so it must not answer the question. It previously
// returned "no errors", which rendered a green "syntax is valid" on the one page
// where being wrong locks an operator out of their own host.
func TestDemoSend_ValidateCustom_ReportsUnavailable(t *testing.T) {
	c := NewDemoClient()
	for _, rules := range [][]string{
		{"tcp dport 8443 accept"},       // valid nftables
		{"this is not nftables at all"}, // not
		{},                              // nothing at all
	} {
		errs, err := c.ValidateCustom(rules)
		if err == nil {
			t.Errorf("ValidateCustom(%v) reported a verdict it cannot have; errs=%v", rules, errs)
		}
	}
}

// ── Audit log gets entries on saves ──────────────────────────────────────

func TestDemoSend_AuditLogAccumulates(t *testing.T) {
	c := NewDemoClient()
	before, _ := c.GetLog()
	beforeLen := len(before)

	_ = c.SaveRules("tcp", []shared.PortRule{{Port: "1234"}})
	_ = c.SaveOptions(shared.FirewallOptions{ICMPFlood: true})

	after, _ := c.GetLog()
	if len(after) != beforeLen+2 {
		t.Errorf("expected %d audit entries (was %d, +2 saves), got %d", beforeLen+2, beforeLen, len(after))
	}
	// Newest entry should be the options save (most recent)
	if after[0].Action != "options_saved" {
		t.Errorf("expected newest entry to be options_saved, got %s", after[0].Action)
	}
}

// ── Export / Import round-trip ───────────────────────────────────────────

func TestDemoSend_ExportImportRoundTrip(t *testing.T) {
	c := NewDemoClient()
	// Capture seeded state.
	exported, err := c.ExportRules()
	if err != nil {
		t.Fatalf("ExportRules: %v", err)
	}

	// Mutate Staged so import has something to do.
	_ = c.SaveRules("tcp", []shared.PortRule{{Port: "7777"}})

	// Import the exported (original) state — this replaces Staged.
	if err := c.ImportRules(exported); err != nil {
		t.Fatalf("ImportRules: %v", err)
	}

	state, _ := c.GetRules()
	// Imported Staged should match the originally-Current TCP rule set.
	if len(state.Staged.TCP) == 0 {
		t.Error("expected staged TCP to be restored from import")
	}
	if state.Staged.TCP[0].Port == "7777" {
		t.Error("import should have replaced the post-save state")
	}
}

// The demo is how most people meet easywall, so what it records has to be what
// the product records. Its saves used to write an empty detail while its seeded
// history showed detailed ones — so the first thing a visitor changed produced
// an entry poorer than every entry above it.
func TestDemoSend_AuditEntriesSayWhatChanged(t *testing.T) {
	c := NewDemoClient()

	before, err := c.GetRules()
	if err != nil {
		t.Fatal(err)
	}
	updated := append(append([]string{}, before.Staged.Blacklist...), "198.51.100.77")
	if err := c.SaveRules("blacklist", updated); err != nil {
		t.Fatal(err)
	}

	entries, err := c.GetLog()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected an audit entry")
	}
	if got := entries[0].Detail; !strings.Contains(got, "198.51.100.77") {
		t.Errorf("the entry must name the address that was added, got %q", got)
	}

	settings := shared.NetworkSettings{
		IPv6:   shared.IPv6Config{Mode: shared.IPv6Block},
		Docker: shared.DockerConfig{Enabled: true},
	}
	if err := c.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	entries, err = c.GetLog()
	if err != nil {
		t.Fatal(err)
	}
	if got := entries[0].Detail; !strings.Contains(got, "mode") {
		t.Errorf("the entry must name the field that moved, got %q", got)
	}
}

func TestDemoSend_RejectsAnOutOfRangeAcceptanceDuration(t *testing.T) {
	c := NewDemoClient()
	for _, dur := range []int{0, 1, 9, 3601} {
		payload, _ := json.Marshal(shared.SystemSettings{
			Acceptance: shared.AcceptanceConfig{Enabled: true, Duration: dur},
		})
		resp, _ := c.Send(shared.Command{Type: shared.CmdSaveSystem, Payload: payload})
		if resp.Success {
			t.Errorf("duration %d is outside the permitted range and must be refused", dur)
		}
	}
}

// The demo has to refuse what the core refuses. It used to check only that the
// payload was JSON of the right shape, so an address the real firewall would
// never accept was stored and shown as blocked.
func TestDemoSend_RejectsWhatTheCoreWouldReject(t *testing.T) {
	c := NewDemoClient()

	before, err := c.GetRules()
	if err != nil {
		t.Fatal(err)
	}

	if err := c.SaveRules("blacklist", []string{"192.168.1.999"}); err == nil {
		t.Error("a malformed address must be refused")
	}
	if err := c.SaveRules("tcp", []shared.PortRule{{Port: "80abc"}}); err == nil {
		t.Error("a port with trailing rubbish must be refused")
	}
	if err := c.SaveRules("forwarding", []shared.ForwardingRule{{Protocol: "sctp", SourcePort: 1, DestPort: 2}}); err == nil {
		t.Error("a protocol other than tcp or udp must be refused")
	}

	after, err := c.GetRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Staged.Blacklist) != len(before.Staged.Blacklist) {
		t.Error("a refused save must leave the staged set untouched")
	}

	// And what the core accepts still goes through, comments included.
	if err := c.SaveRules("blacklist", []string{"# a note", "", "192.0.2.7"}); err != nil {
		t.Errorf("a valid list with comments must be accepted: %v", err)
	}
}
