package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
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

// Tests for json.Unmarshal type-mismatch errors in each SaveStaged case.
// Passing a plain string marshals to a JSON string, which cannot be
// unmarshaled into a slice type — exercises the return-err branch in each case.

func TestSaveStaged_TCP_TypeMismatch(t *testing.T) {
	store, _ := newTempStore(t)
	// "not a list" marshals to JSON string, can't unmarshal into []PortRule
	if err := store.SaveStaged("tcp", "not a list"); err == nil {
		t.Error("expected error for wrong type in tcp case")
	}
}

func TestSaveStaged_UDP_TypeMismatch(t *testing.T) {
	store, _ := newTempStore(t)
	if err := store.SaveStaged("udp", "not a list"); err == nil {
		t.Error("expected error for wrong type in udp case")
	}
}

func TestSaveStaged_Blacklist_TypeMismatch(t *testing.T) {
	store, _ := newTempStore(t)
	// []int{1,2,3} marshals to [1,2,3], can't unmarshal into []string
	if err := store.SaveStaged("blacklist", []int{1, 2, 3}); err == nil {
		t.Error("expected error for wrong type in blacklist case")
	}
}

func TestSaveStaged_Whitelist_TypeMismatch(t *testing.T) {
	store, _ := newTempStore(t)
	if err := store.SaveStaged("whitelist", []int{1, 2, 3}); err == nil {
		t.Error("expected error for wrong type in whitelist case")
	}
}

func TestSaveStaged_Forwarding_TypeMismatch(t *testing.T) {
	store, _ := newTempStore(t)
	if err := store.SaveStaged("forwarding", "not a list"); err == nil {
		t.Error("expected error for wrong type in forwarding case")
	}
}

func TestSaveStaged_Custom_TypeMismatch(t *testing.T) {
	store, _ := newTempStore(t)
	if err := store.SaveStaged("custom", []int{1, 2, 3}); err == nil {
		t.Error("expected error for wrong type in custom case")
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

// --- validatePortRule ---

// --- validateRules ---

// --- Error paths in store operations ---

func TestNewRulesStore_InvalidDir(t *testing.T) {
	_, err := NewRulesStore("/nonexistent/dir/rules.json")
	if err == nil {
		t.Error("expected error when directory does not exist")
	}
}

func TestGetState_MissingFile(t *testing.T) {
	store := &RulesStore{path: "/nonexistent/rules.json"}
	_, err := store.GetState()
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestGetState_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")
	_ = os.WriteFile(path, []byte("not valid json"), 0600)
	store := &RulesStore{path: path}
	_, err := store.GetState()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestHasPendingChanges_ReadError(t *testing.T) {
	store := &RulesStore{path: "/nonexistent/rules.json"}
	_, err := store.HasPendingChanges()
	if err == nil {
		t.Error("expected error when rules file is missing")
	}
}

func TestSaveStagedGetStateError(t *testing.T) {
	store := &RulesStore{path: "/nonexistent/rules.json"}
	err := store.SaveStaged("tcp", []shared.PortRule{{Port: "80"}})
	if err == nil {
		t.Error("expected error when file is missing")
	}
}

func TestBackupCurrent_GetStateError(t *testing.T) {
	store := &RulesStore{path: "/nonexistent/rules.json"}
	err := store.BackupCurrent()
	if err == nil {
		t.Error("expected error when file is missing")
	}
}

func TestPromoteStaged_GetStateError(t *testing.T) {
	store := &RulesStore{path: "/nonexistent/rules.json"}
	err := store.PromoteStaged()
	if err == nil {
		t.Error("expected error when file is missing")
	}
}

func TestRollback_GetStateError(t *testing.T) {
	store := &RulesStore{path: "/nonexistent/rules.json"}
	err := store.Rollback()
	if err == nil {
		t.Error("expected error when file is missing")
	}
}

func TestExportCurrent_GetStateError(t *testing.T) {
	store := &RulesStore{path: "/nonexistent/rules.json"}
	_, err := store.ExportCurrent()
	if err == nil {
		t.Error("expected error when file is missing")
	}
}

func TestImportRules_GetStateError(t *testing.T) {
	store := &RulesStore{path: "/nonexistent/rules.json"}
	err := store.ImportRules([]byte(`{"tcp":[],"udp":[],"blacklist":[],"whitelist":[],"forwarding":[],"custom":[]}`))
	if err == nil {
		t.Error("expected error when file is missing")
	}
}

func TestSaveInvalidDir(t *testing.T) {
	store := &RulesStore{path: "/nonexistent/path/rules.json"}
	err := store.save(emptyState())
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

// TestSave_RenameError triggers the atomic-rename failure path by creating a
// directory at the target path so os.Rename(tmp, dir) returns EISDIR.
func TestSave_RenameError(t *testing.T) {
	baseDir := t.TempDir()
	// Create a directory with the same name as the target file — Rename will fail.
	targetPath := filepath.Join(baseDir, "rules.json")
	if err := os.Mkdir(targetPath, 0755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	store := &RulesStore{path: targetPath}
	err := store.save(emptyState())
	if err == nil {
		t.Error("expected error when rename target is a directory")
	}
}

// --- WriteAuditLog ---

func TestWriteAuditLog_InvalidPath(t *testing.T) {
	// Should silently fail without panic
	WriteAuditLog("/nonexistent/path/audit.log", "apply", "tcp", "", "admin")
}

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

// SaveStaged validates before persisting. ImportRules always did and SaveStaged
// never did, so the same malformed address was rejected when it arrived in a
// file and accepted when it arrived from the web form — then stored, shown as
// blocked, and silently skipped at apply time.
func TestSaveStaged_RejectsInvalidAddresses(t *testing.T) {
	cases := []struct {
		name     string
		ruleType string
		rules    interface{}
	}{
		{"blacklist octet out of range", "blacklist", []string{"192.168.1.999"}},
		{"blacklist prefix out of range", "blacklist", []string{"10.0.0.0/33"}},
		{"blacklist hostname", "blacklist", []string{"example.com"}},
		{"whitelist malformed", "whitelist", []string{"10.0.0."}},
		{"tcp port out of range", "tcp", []shared.PortRule{{Port: "70000"}}},
		{"forwarding bad protocol", "forwarding", []shared.ForwardingRule{
			{Protocol: "sctp", SourcePort: 80, DestPort: 8080}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewRulesStore(t.TempDir() + "/rules.json")
			if err != nil {
				t.Fatalf("NewRulesStore: %v", err)
			}
			if err := store.SaveStaged(tc.ruleType, tc.rules); err == nil {
				t.Error("SaveStaged accepted a value that can never become a rule")
			}

			// And nothing may have been written.
			state, err := store.GetState()
			if err != nil {
				t.Fatalf("GetState: %v", err)
			}
			if len(state.Staged.Blacklist)+len(state.Staged.Whitelist)+
				len(state.Staged.TCP)+len(state.Staged.Forwarding) != 0 {
				t.Errorf("a rejected save still persisted something: %+v", state.Staged)
			}
		})
	}
}

func TestSaveStaged_AcceptsValidAddresses(t *testing.T) {
	store, err := NewRulesStore(t.TempDir() + "/rules.json")
	if err != nil {
		t.Fatalf("NewRulesStore: %v", err)
	}
	valid := []string{"192.0.2.1", "198.51.100.0/24", "2001:db8::1", "2001:db8::/32"}
	if err := store.SaveStaged("blacklist", valid); err != nil {
		t.Fatalf("SaveStaged rejected valid entries: %v", err)
	}
	state, err := store.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if len(state.Staged.Blacklist) != len(valid) {
		t.Errorf("expected %d entries, got %d", len(valid), len(state.Staged.Blacklist))
	}
}

// Ports are parsed strictly. fmt.Sscanf stops at the first character it cannot
// read and reports what it managed, so "80abc" validated as port 80 and
// "80 90" as port 80 — the rule list showed one thing and the firewall
// enforced another.
