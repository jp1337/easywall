package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jpylypiw/easywall/internal/shared"
)

func newTempStore(t *testing.T) (*RulesStore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")
	store, err := NewRulesStore(path)
	if err != nil {
		t.Fatalf("NewRulesStore: %v", err)
	}
	return store, path
}

func TestNewRulesStore_CreatesFile(t *testing.T) {
	_, path := newTempStore(t)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected rules file to exist: %v", err)
	}
}

func TestNewRulesStore_OpensExisting(t *testing.T) {
	store, path := newTempStore(t)
	// add a staged entry then reopen
	err := store.SaveStaged("tcp", []shared.PortRule{{Port: "80"}})
	if err != nil {
		t.Fatal(err)
	}
	store2, err := NewRulesStore(path)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store2.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Staged.TCP) != 1 || state.Staged.TCP[0].Port != "80" {
		t.Errorf("reopened store lost data, got %+v", state.Staged.TCP)
	}
}

func TestGetState_EmptyInitial(t *testing.T) {
	store, _ := newTempStore(t)
	state, err := store.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Current.TCP) != 0 || len(state.Staged.TCP) != 0 {
		t.Error("expected empty initial state")
	}
}

func TestSaveStaged_TCP(t *testing.T) {
	store, _ := newTempStore(t)
	rules := []shared.PortRule{{Port: "443", Description: "HTTPS"}}
	if err := store.SaveStaged("tcp", rules); err != nil {
		t.Fatal(err)
	}
	state, _ := store.GetState()
	if len(state.Staged.TCP) != 1 || state.Staged.TCP[0].Port != "443" {
		t.Errorf("unexpected staged TCP: %+v", state.Staged.TCP)
	}
	// current must not change
	if len(state.Current.TCP) != 0 {
		t.Error("SaveStaged must not modify current")
	}
}

func TestSaveStaged_UDP(t *testing.T) {
	store, _ := newTempStore(t)
	if err := store.SaveStaged("udp", []shared.PortRule{{Port: "53"}}); err != nil {
		t.Fatal(err)
	}
	state, _ := store.GetState()
	if len(state.Staged.UDP) != 1 {
		t.Errorf("expected 1 UDP rule, got %d", len(state.Staged.UDP))
	}
}

func TestSaveStaged_Blacklist(t *testing.T) {
	store, _ := newTempStore(t)
	if err := store.SaveStaged("blacklist", []string{"1.2.3.4", "10.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	state, _ := store.GetState()
	if len(state.Staged.Blacklist) != 2 {
		t.Errorf("expected 2 blacklist entries, got %d", len(state.Staged.Blacklist))
	}
}

func TestSaveStaged_Whitelist(t *testing.T) {
	store, _ := newTempStore(t)
	if err := store.SaveStaged("whitelist", []string{"192.168.1.0/24"}); err != nil {
		t.Fatal(err)
	}
	state, _ := store.GetState()
	if len(state.Staged.Whitelist) != 1 {
		t.Errorf("expected 1 whitelist entry, got %d", len(state.Staged.Whitelist))
	}
}

func TestSaveStaged_Forwarding(t *testing.T) {
	store, _ := newTempStore(t)
	rules := []shared.ForwardingRule{{Protocol: "tcp", SourcePort: 8080, DestPort: 80}}
	if err := store.SaveStaged("forwarding", rules); err != nil {
		t.Fatal(err)
	}
	state, _ := store.GetState()
	if len(state.Staged.Forwarding) != 1 {
		t.Errorf("expected 1 forwarding rule, got %d", len(state.Staged.Forwarding))
	}
}

func TestSaveStaged_Custom(t *testing.T) {
	store, _ := newTempStore(t)
	if err := store.SaveStaged("custom", []string{"add rule inet easywall input ip saddr 1.2.3.4 drop"}); err != nil {
		t.Fatal(err)
	}
	state, _ := store.GetState()
	if len(state.Staged.Custom) != 1 {
		t.Errorf("expected 1 custom rule, got %d", len(state.Staged.Custom))
	}
}

func TestSaveStaged_UnknownType(t *testing.T) {
	store, _ := newTempStore(t)
	err := store.SaveStaged("bogus", []string{"x"})
	if err == nil {
		t.Error("expected error for unknown rule type")
	}
}

func TestHasPendingChanges_False(t *testing.T) {
	store, _ := newTempStore(t)
	pending, err := store.HasPendingChanges()
	if err != nil {
		t.Fatal(err)
	}
	if pending {
		t.Error("fresh store must not report pending changes")
	}
}

func TestHasPendingChanges_True(t *testing.T) {
	store, _ := newTempStore(t)
	_ = store.SaveStaged("tcp", []shared.PortRule{{Port: "80"}})
	pending, err := store.HasPendingChanges()
	if err != nil {
		t.Fatal(err)
	}
	if !pending {
		t.Error("expected pending changes after SaveStaged")
	}
}

func TestPromoteStaged(t *testing.T) {
	store, _ := newTempStore(t)
	_ = store.SaveStaged("tcp", []shared.PortRule{{Port: "8080"}})
	if err := store.PromoteStaged(); err != nil {
		t.Fatal(err)
	}
	state, _ := store.GetState()
	if len(state.Current.TCP) != 1 || state.Current.TCP[0].Port != "8080" {
		t.Errorf("PromoteStaged did not update current: %+v", state.Current.TCP)
	}
}

func TestBackupCurrent(t *testing.T) {
	store, _ := newTempStore(t)
	_ = store.SaveStaged("tcp", []shared.PortRule{{Port: "22"}})
	_ = store.PromoteStaged()
	if err := store.BackupCurrent(); err != nil {
		t.Fatal(err)
	}
	state, _ := store.GetState()
	if len(state.Backup.TCP) != 1 || state.Backup.TCP[0].Port != "22" {
		t.Errorf("BackupCurrent did not copy current to backup: %+v", state.Backup.TCP)
	}
}

func TestRollback(t *testing.T) {
	store, _ := newTempStore(t)
	// set up: current=22, staged=80, backup=22
	_ = store.SaveStaged("tcp", []shared.PortRule{{Port: "22"}})
	_ = store.PromoteStaged()
	_ = store.BackupCurrent()
	_ = store.SaveStaged("tcp", []shared.PortRule{{Port: "80"}})
	_ = store.PromoteStaged()

	if err := store.Rollback(); err != nil {
		t.Fatal(err)
	}
	state, _ := store.GetState()
	if len(state.Current.TCP) != 1 || state.Current.TCP[0].Port != "22" {
		t.Errorf("Rollback did not restore from backup: %+v", state.Current.TCP)
	}
	if len(state.Staged.TCP) != 1 || state.Staged.TCP[0].Port != "22" {
		t.Errorf("Rollback did not reset staged: %+v", state.Staged.TCP)
	}
}

func TestExportCurrent(t *testing.T) {
	store, _ := newTempStore(t)
	_ = store.SaveStaged("tcp", []shared.PortRule{{Port: "443"}})
	_ = store.PromoteStaged()

	data, err := store.ExportCurrent()
	if err != nil {
		t.Fatal(err)
	}
	var rules shared.Rules
	if err := json.Unmarshal(data, &rules); err != nil {
		t.Fatalf("exported data is not valid JSON: %v", err)
	}
	if len(rules.TCP) != 1 || rules.TCP[0].Port != "443" {
		t.Errorf("unexpected exported rules: %+v", rules)
	}
}

func TestImportRules_Valid(t *testing.T) {
	store, _ := newTempStore(t)
	data := `{"tcp":[{"port":"80","description":"HTTP"}],"udp":[],"blacklist":[],"whitelist":[],"forwarding":[],"custom":[]}`
	if err := store.ImportRules([]byte(data)); err != nil {
		t.Fatalf("ImportRules failed: %v", err)
	}
	state, _ := store.GetState()
	if len(state.Staged.TCP) != 1 || state.Staged.TCP[0].Port != "80" {
		t.Errorf("imported data not in staged: %+v", state.Staged.TCP)
	}
}

func TestImportRules_InvalidJSON(t *testing.T) {
	store, _ := newTempStore(t)
	if err := store.ImportRules([]byte("not json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestImportRules_InvalidPort(t *testing.T) {
	store, _ := newTempStore(t)
	data := `{"tcp":[{"port":"99999"}],"udp":[],"blacklist":[],"whitelist":[],"forwarding":[],"custom":[]}`
	if err := store.ImportRules([]byte(data)); err == nil {
		t.Error("expected validation error for port 99999")
	}
}

// --- validateIPOrCIDR ---

func TestValidateIPOrCIDR(t *testing.T) {
	valid := []string{"192.168.1.1", "10.0.0.0/8", "::1", "2001:db8::/32"}
	for _, s := range valid {
		if err := validateIPOrCIDR(s); err != nil {
			t.Errorf("expected %q to be valid: %v", s, err)
		}
	}
	invalid := []string{"", "notanip", "256.1.2.3", "10.0.0.0/33"}
	for _, s := range invalid {
		if err := validateIPOrCIDR(s); err == nil {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

// --- validatePortRule ---

func TestValidatePortRule(t *testing.T) {
	valid := []shared.PortRule{
		{Port: "1"}, {Port: "65535"}, {Port: "80"}, {Port: "8000:9000"},
	}
	for _, r := range valid {
		if err := validatePortRule(r); err != nil {
			t.Errorf("expected port %q to be valid: %v", r.Port, err)
		}
	}
	invalid := []shared.PortRule{
		{Port: ""},
		{Port: "0"},
		{Port: "65536"},
		{Port: "abc"},
		{Port: "9000:8000"}, // end < start
		{Port: "0:100"},
	}
	for _, r := range invalid {
		if err := validatePortRule(r); err == nil {
			t.Errorf("expected port %q to be invalid", r.Port)
		}
	}
}

// --- validateRules ---

func TestValidateRules_InvalidForwardingProtocol(t *testing.T) {
	r := shared.Rules{
		TCP: []shared.PortRule{}, UDP: []shared.PortRule{},
		Blacklist: []string{}, Whitelist: []string{}, Custom: []string{},
		Forwarding: []shared.ForwardingRule{{Protocol: "icmp", SourcePort: 80, DestPort: 8080}},
	}
	if err := validateRules(r); err == nil {
		t.Error("expected error for invalid forwarding protocol")
	}
}

func TestValidateRules_InvalidForwardingPort(t *testing.T) {
	r := shared.Rules{
		TCP: []shared.PortRule{}, UDP: []shared.PortRule{},
		Blacklist: []string{}, Whitelist: []string{}, Custom: []string{},
		Forwarding: []shared.ForwardingRule{{Protocol: "tcp", SourcePort: 0, DestPort: 80}},
	}
	if err := validateRules(r); err == nil {
		t.Error("expected error for source port 0")
	}
}

// --- WriteAuditLog ---

func TestWriteAuditLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")
	WriteAuditLog(logPath, "apply", "tcp", "port 80", "admin")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("audit log is empty")
	}
	// check it's valid JSON (one line)
	var entry map[string]string
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil {
		t.Errorf("audit log entry is not valid JSON: %v — got: %s", err, data)
	}
	if entry["action"] != "apply" {
		t.Errorf("unexpected action: %s", entry["action"])
	}
}
