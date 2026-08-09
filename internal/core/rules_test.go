package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// A rollback restores what is enforced and keeps what you were writing.
//
// This test used to assert the opposite — "Rollback did not reset staged" —
// which is what the code did and the exact opposite of what the apply-flow
// diagram promises on four pages: "Previous rules are back. Nothing staged was
// lost."
func TestRollback_RestoresCurrentAndKeepsTheStagedEdits(t *testing.T) {
	store, _ := newTempStore(t)
	// The sequence an apply performs: current=22, backup=22, then the edit is
	// staged and promoted, so current=staged=80.
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
		t.Errorf("the enforced set must come back from the backup: %+v", state.Current.TCP)
	}
	if len(state.Staged.TCP) != 1 || state.Staged.TCP[0].Port != "80" {
		t.Errorf("the edits that were applied must still be staged, ready to be corrected: %+v",
			state.Staged.TCP)
	}

	// And the interface must say there is something pending, because there is.
	pending, err := store.HasPendingChanges()
	if err != nil {
		t.Fatal(err)
	}
	if !pending {
		t.Error("after a rollback the staged edits differ from what is enforced")
	}
}

func TestExportStaged(t *testing.T) {
	store, _ := newTempStore(t)
	_ = store.SaveStaged("tcp", []shared.PortRule{{Port: "443"}})
	_ = store.PromoteStaged()

	data, err := store.ExportStaged()
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

func TestExportStaged_GetStateError(t *testing.T) {
	store := &RulesStore{path: "/nonexistent/rules.json"}
	_, err := store.ExportStaged()
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

// A log that cannot be written must not take the daemon down with it — an audit
// entry is a record of something that already happened. It must also not be
// mistaken for one that was written: the failure is logged now rather than
// discarded.
func TestWriteAuditLog_UnwritablePathWritesNothingAndDoesNotPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "audit.log")
	WriteAuditLog(path, "apply", "tcp", "", "admin")

	if _, err := os.Stat(path); err == nil {
		t.Error("nothing should have been created under a directory that does not exist")
	}
}

// What it writes has to be what the reader parses. The writer used fmt's %q,
// which is not a JSON encoder, while the reader has always used encoding/json
// and skips a line it cannot decode.
func TestWriteAuditLog_RoundTripsThroughTheReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	WriteAuditLog(path, "rules_saved", "blacklist", `added 203.0.113.7 "quoted", removed 192.0.2.1`, "web")

	entries, err := readAuditLog(path, 10)
	if err != nil {
		t.Fatalf("readAuditLog: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Action != "rules_saved" || e.RuleType != "blacklist" || e.User != "web" {
		t.Errorf("fields did not survive the round trip: %+v", e)
	}
	if !strings.Contains(e.Detail, `"quoted"`) {
		t.Errorf("the detail lost its quoting: %q", e.Detail)
	}
	if e.Time == "" {
		t.Error("entries must be timestamped")
	}
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

// Two saves that arrive together must both survive.
//
// Every write is a read-modify-write of one file holding all six lists, and the
// daemon handles each socket connection on its own goroutine. Without a lock,
// both saves were built on the same read and the second wrote the first one
// away: measured at 187 of 200 trials, silently, with the interface reporting
// "Changes saved" to both. Two browser tabs is all it takes.
func TestSaveStaged_ConcurrentSavesDoNotLoseEachOther(t *testing.T) {
	const trials = 100
	lost := 0

	for i := 0; i < trials; i++ {
		store, _ := newTempStore(t)

		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_ = store.SaveStaged("blacklist", []string{"192.0.2.1"})
		}()
		go func() {
			defer wg.Done()
			<-start
			_ = store.SaveStaged("whitelist", []string{"203.0.113.1"})
		}()
		close(start)
		wg.Wait()

		state, err := store.GetState()
		if err != nil {
			t.Fatalf("GetState: %v", err)
		}
		if len(state.Staged.Blacklist) == 0 || len(state.Staged.Whitelist) == 0 {
			lost++
		}
	}

	if lost > 0 {
		t.Errorf("%d of %d concurrent saves discarded the other change", lost, trials)
	}
}

// The same for the sequence an apply performs, run against saves arriving from
// another connection.
func TestRulesStore_ApplySequenceIsAtomicAgainstConcurrentSaves(t *testing.T) {
	store, _ := newTempStore(t)
	if err := store.SaveStaged("tcp", []shared.PortRule{{Port: "22"}}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = store.BackupCurrent()
			_ = store.PromoteStaged()
		}()
		go func() {
			defer wg.Done()
			_ = store.SaveStaged("blacklist", []string{"198.51.100.1"})
		}()
	}
	wg.Wait()

	// Whatever the interleaving, the file must still be a valid document.
	state, err := store.GetState()
	if err != nil {
		t.Fatalf("the rules file did not survive concurrent access: %v", err)
	}
	if len(state.Staged.TCP) != 1 || state.Staged.TCP[0].Port != "22" {
		t.Errorf("the staged port list was corrupted: %+v", state.Staged.TCP)
	}
}

// Export and import have to be a pair: import replaces the staged set, so
// export has to read it, or the round trip loses exactly the edits an operator
// exported to protect. export-import.md offers this as what to do "before a
// risky change" — advice worth taking only if the file holds the change.
func TestExportImport_RoundTripsTheStagedSetIncludingComments(t *testing.T) {
	store, _ := newTempStore(t)

	// Something applied and live, and a different set staged on top of it.
	if err := store.SaveStaged("tcp", []shared.PortRule{{Port: "22"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.PromoteStaged(); err != nil {
		t.Fatal(err)
	}
	staged := []string{"# from the abuse report", "192.0.2.42", "", "198.51.100.0/24"}
	if err := store.SaveStaged("blacklist", staged); err != nil {
		t.Fatal(err)
	}

	data, err := store.ExportStaged()
	if err != nil {
		t.Fatalf("ExportStaged: %v", err)
	}

	var exported shared.Rules
	if err := json.Unmarshal(data, &exported); err != nil {
		t.Fatalf("the export is not valid JSON: %v", err)
	}
	if len(exported.Blacklist) != len(staged) {
		t.Fatalf("the export lost lines: %+v", exported.Blacklist)
	}
	if exported.Blacklist[0] != "# from the abuse report" {
		t.Errorf("the export dropped the comment: %+v", exported.Blacklist)
	}

	// Importing it back into a fresh store reproduces what was staged.
	other, _ := newTempStore(t)
	if err := other.ImportRules(data); err != nil {
		t.Fatalf("the export must import: %v", err)
	}
	state, err := other.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Staged.Blacklist) != len(staged) {
		t.Errorf("the round trip lost lines: %+v", state.Staged.Blacklist)
	}
	for i := range staged {
		if state.Staged.Blacklist[i] != staged[i] {
			t.Errorf("line %d: got %q, want %q", i, state.Staged.Blacklist[i], staged[i])
		}
	}
}
