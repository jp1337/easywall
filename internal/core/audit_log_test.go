package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadAuditLog_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")
	_ = os.WriteFile(path, []byte{}, 0640)

	entries, err := readAuditLog(path, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestReadAuditLog_NotExist(t *testing.T) {
	_, err := readAuditLog("/nonexistent/audit.log", 10)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestReadAuditLog_ParsesEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	content := `{"time":"2026-04-27T10:00:00Z","action":"rules_saved","rule_type":"tcp","detail":"","user":"web"}
{"time":"2026-04-27T10:01:00Z","action":"options_saved","rule_type":"","detail":"","user":"web"}
`
	if err := os.WriteFile(path, []byte(content), 0640); err != nil {
		t.Fatal(err)
	}

	entries, err := readAuditLog(path, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Entries should be newest-first
	if entries[0].Action != "options_saved" {
		t.Errorf("expected first entry to be options_saved (newest), got %s", entries[0].Action)
	}
	if entries[1].Action != "rules_saved" {
		t.Errorf("expected second entry to be rules_saved (oldest), got %s", entries[1].Action)
	}
}

func TestReadAuditLog_LimitRespected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	var content string
	for i := 0; i < 10; i++ {
		content += `{"time":"2026-04-27T10:00:00Z","action":"rules_saved","rule_type":"tcp","detail":"","user":"web"}` + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0640); err != nil {
		t.Fatal(err)
	}

	entries, err := readAuditLog(path, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries (limit), got %d", len(entries))
	}
}

func TestReadAuditLog_SkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	content := `{"time":"2026-04-27T10:00:00Z","action":"rules_saved","rule_type":"tcp","detail":"","user":"web"}
not valid json
{"time":"2026-04-27T10:01:00Z","action":"settings_saved","rule_type":"","detail":"","user":"web"}
`
	if err := os.WriteFile(path, []byte(content), 0640); err != nil {
		t.Fatal(err)
	}

	entries, err := readAuditLog(path, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 valid entries, got %d", len(entries))
	}
}
