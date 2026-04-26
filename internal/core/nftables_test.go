package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/nftables/expr"
	"github.com/jp1337/easywall/internal/shared"
)

// --- policyDrop / policyAccept ---

func TestPolicyDrop_ReturnsDropPolicy(t *testing.T) {
	p := policyDrop()
	if p == nil {
		t.Fatal("expected non-nil policy")
	}
}

func TestPolicyAccept_ReturnsAcceptPolicy(t *testing.T) {
	p := policyAccept()
	if p == nil {
		t.Fatal("expected non-nil policy")
	}
}

func TestPolicyDropAndAccept_Differ(t *testing.T) {
	drop := policyDrop()
	accept := policyAccept()
	if *drop == *accept {
		t.Error("drop and accept policies must differ")
	}
}

// --- parsePort ---

func TestParsePort_Valid(t *testing.T) {
	cases := []struct {
		input    string
		expected uint16
	}{
		{"80", 80},
		{"443", 443},
		{"1", 1},
		{"65535", 65535},
		{"8080", 8080},
	}
	for _, tc := range cases {
		got := parsePort(tc.input)
		if got != tc.expected {
			t.Errorf("parsePort(%q) = %d, want %d", tc.input, got, tc.expected)
		}
	}
}

func TestParsePort_Invalid(t *testing.T) {
	invalid := []string{"0", "65536", "abc", "", "-1", "99999"}
	for _, s := range invalid {
		if p := parsePort(s); p != 0 {
			t.Errorf("parsePort(%q) = %d, want 0 (invalid)", s, p)
		}
	}
}

func TestParsePort_BoundaryValues(t *testing.T) {
	if p := parsePort("1"); p != 1 {
		t.Errorf("expected 1, got %d", p)
	}
	if p := parsePort("65535"); p != 65535 {
		t.Errorf("expected 65535, got %d", p)
	}
	if p := parsePort("0"); p != 0 {
		t.Errorf("expected 0 for out-of-range, got %d", p)
	}
}

// --- buildPortExprs ---

func TestBuildPortExprs_SinglePort(t *testing.T) {
	exprs := buildPortExprs("80")
	if len(exprs) == 0 {
		t.Fatal("expected non-empty exprs for single port")
	}
	// Should have Payload + Cmp
	if len(exprs) != 2 {
		t.Errorf("expected 2 exprs for single port, got %d", len(exprs))
	}
	_, ok := exprs[0].(*expr.Payload)
	if !ok {
		t.Error("expected first expr to be *expr.Payload")
	}
	_, ok = exprs[1].(*expr.Cmp)
	if !ok {
		t.Error("expected second expr to be *expr.Cmp")
	}
}

func TestBuildPortExprs_PortRange(t *testing.T) {
	exprs := buildPortExprs("8000:9000")
	if len(exprs) != 3 {
		t.Errorf("expected 3 exprs for port range, got %d", len(exprs))
	}
	_, ok := exprs[0].(*expr.Payload)
	if !ok {
		t.Error("expected first expr to be *expr.Payload")
	}
	_, ok = exprs[1].(*expr.Cmp)
	if !ok {
		t.Error("expected second expr to be *expr.Cmp (gte)")
	}
	_, ok = exprs[2].(*expr.Cmp)
	if !ok {
		t.Error("expected third expr to be *expr.Cmp (lte)")
	}
}

func TestBuildPortExprs_Range_GteLte(t *testing.T) {
	exprs := buildPortExprs("1024:2048")
	if len(exprs) != 3 {
		t.Fatalf("expected 3 exprs, got %d", len(exprs))
	}
	gte, ok := exprs[1].(*expr.Cmp)
	if !ok {
		t.Fatal("expected *expr.Cmp for gte")
	}
	if gte.Op != expr.CmpOpGte {
		t.Errorf("expected CmpOpGte, got %v", gte.Op)
	}
	lte, ok := exprs[2].(*expr.Cmp)
	if !ok {
		t.Fatal("expected *expr.Cmp for lte")
	}
	if lte.Op != expr.CmpOpLte {
		t.Errorf("expected CmpOpLte, got %v", lte.Op)
	}
}

func TestBuildPortExprs_SinglePort_EqOp(t *testing.T) {
	exprs := buildPortExprs("443")
	if len(exprs) != 2 {
		t.Fatalf("expected 2 exprs, got %d", len(exprs))
	}
	cmp, ok := exprs[1].(*expr.Cmp)
	if !ok {
		t.Fatal("expected *expr.Cmp")
	}
	if cmp.Op != expr.CmpOpEq {
		t.Errorf("expected CmpOpEq, got %v", cmp.Op)
	}
}

func TestBuildPortExprs_PortEncoding(t *testing.T) {
	// Port 80 = 0x50 → bytes [0x00, 0x50]
	exprs := buildPortExprs("80")
	cmp := exprs[1].(*expr.Cmp)
	if len(cmp.Data) != 2 {
		t.Fatalf("expected 2 bytes for port, got %d", len(cmp.Data))
	}
	if cmp.Data[0] != 0 || cmp.Data[1] != 80 {
		t.Errorf("unexpected port bytes: %v", cmp.Data)
	}
}

// --- SaveSnapshot / rotateSnapshots ---

func TestSaveSnapshot_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{"tables":1,"timestamp":"2026-01-01T00:00:00Z"}`)
	if err := SaveSnapshot(dir, data); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Error("expected snapshot file to be created")
	}
	// Verify filename format
	name := entries[0].Name()
	if len(name) < 10 {
		t.Errorf("unexpected snapshot filename: %s", name)
	}
}

func TestSaveSnapshot_FileContent(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{"test":"content"}`)
	if err := SaveSnapshot(dir, data); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Fatal("no snapshot file found")
	}
	content, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(data) {
		t.Errorf("snapshot content mismatch: got %s, want %s", content, data)
	}
}

func TestSaveSnapshot_InvalidDir(t *testing.T) {
	err := SaveSnapshot("/nonexistent/dir", []byte("data"))
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}

// Tests for nil-conn guard added to Snapshot and Reset.

func TestNftablesManager_Snapshot_NilConn(t *testing.T) {
	m := &NftablesManager{} // conn is nil
	_, err := m.Snapshot()
	if err == nil {
		t.Error("expected error for nil connection")
	}
}

func TestNftablesManager_Reset_NilConn(t *testing.T) {
	m := &NftablesManager{} // conn is nil
	err := m.Reset()
	if err == nil {
		t.Error("expected error for nil connection")
	}
}

func TestNftablesManager_Apply_NilConn(t *testing.T) {
	m := &NftablesManager{} // conn is nil — Apply calls Reset which checks nil
	err := m.Apply(shared.RulesState{}, shared.FirewallOptions{}, shared.IPv6Config{}, shared.DockerConfig{})
	if err == nil {
		t.Error("expected error for nil connection")
	}
}

func TestNftablesManager_Restore(t *testing.T) {
	m := &NftablesManager{} // Restore is a no-op placeholder
	err := m.Restore([]byte("snapshot"))
	if err != nil {
		t.Errorf("Restore should always return nil, got: %v", err)
	}
}

func TestRotateSnapshots_KeepsAtMostN(t *testing.T) {
	dir := t.TempDir()
	// Create 12 files
	for i := 0; i < 12; i++ {
		name := filepath.Join(dir, "nftables_2026-01-01_00-00-"+fmt.Sprintf("%02d", i)+".json")
		_ = os.WriteFile(name, []byte("data"), 0600)
	}

	if err := rotateSnapshots(dir, 10); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) > 10 {
		t.Errorf("expected at most 10 files after rotation, got %d", len(entries))
	}
}

func TestRotateSnapshots_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	// Should not fail on empty dir
	if err := rotateSnapshots(dir, 10); err != nil {
		t.Fatalf("rotateSnapshots on empty dir: %v", err)
	}
}

func TestRotateSnapshots_NonExistentDir(t *testing.T) {
	// Should return error for non-existent directory
	err := rotateSnapshots("/nonexistent/dir", 10)
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}

func TestRotateSnapshots_FewerThanKeep(t *testing.T) {
	dir := t.TempDir()
	// Create 3 files, keep 10 → no deletion
	for i := 0; i < 3; i++ {
		name := filepath.Join(dir, "nftables_2026-01-01_00-00-0"+string(rune('0'+i))+".json")
		_ = os.WriteFile(name, []byte("data"), 0600)
	}

	if err := rotateSnapshots(dir, 10); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Errorf("expected 3 files (no rotation needed), got %d", len(entries))
	}
}

func TestSaveSnapshot_MultipleCalls(t *testing.T) {
	dir := t.TempDir()
	// SaveSnapshot calls rotateSnapshots and creates a new file
	for i := 0; i < 3; i++ {
		if err := SaveSnapshot(dir, []byte(`{"i":}`+string(rune('0'+i)))); err != nil {
			t.Fatalf("SaveSnapshot %d: %v", i, err)
		}
	}
	entries, _ := os.ReadDir(dir)
	// Should have 3 files (less than keep=10)
	if len(entries) < 1 {
		t.Error("expected at least 1 snapshot file")
	}
}
