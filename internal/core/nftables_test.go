package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/nftables"
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
	err := m.Apply(shared.RulesState{}, shared.FirewallOptions{}, shared.NetworkSettings{})
	if err == nil {
		t.Error("expected error for nil connection")
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

// --- tableFamilyName ---

func TestTableFamilyName_AllBranches(t *testing.T) {
	cases := []struct {
		family nftables.TableFamily
		want   string
	}{
		{nftables.TableFamilyINet, "inet"},
		{nftables.TableFamilyIPv4, "ip"},
		{nftables.TableFamilyIPv6, "ip6"},
		{nftables.TableFamilyARP, "arp"},
		{nftables.TableFamilyNetdev, "netdev"},
		{nftables.TableFamilyBridge, "bridge"},
		{nftables.TableFamilyUnspecified, "unspecified"},
	}
	for _, tc := range cases {
		got := tableFamilyName(tc.family)
		if got != tc.want {
			t.Errorf("tableFamilyName(%v) = %q, want %q", tc.family, got, tc.want)
		}
	}
}

func TestTableFamilyName_UnknownValue(t *testing.T) {
	// A family value not listed in the switch falls through to "unspecified".
	got := tableFamilyName(nftables.TableFamily(255))
	if got != "unspecified" {
		t.Errorf("expected 'unspecified' for unknown family, got %q", got)
	}
}

// --- applyCustomRules ---

func TestApplyCustomRules_EmptySlice(t *testing.T) {
	m := &NftablesManager{}
	err := m.applyCustomRules([]string{})
	if err != nil {
		t.Errorf("expected nil error for empty rules, got %v", err)
	}
}

func TestApplyCustomRules_OnlyBlanksAndComments(t *testing.T) {
	m := &NftablesManager{}
	// All blank or comment lines → len(cmds)==0 → returns nil without calling nft
	err := m.applyCustomRules([]string{"", "  ", "# a comment", "  # another"})
	if err != nil {
		t.Errorf("expected nil error for blank/comment-only rules, got %v", err)
	}
}

func TestApplyCustomRules_NftNotAvailableOrFails(t *testing.T) {
	if nftAvailable() {
		t.Skip("nft available with kernel access — skipping error path test")
	}
	m := &NftablesManager{}
	// When nft is not available or has no kernel access, this should return an error.
	err := m.applyCustomRules([]string{"tcp dport 80 accept"})
	if err == nil {
		t.Error("expected error when nft is not available or fails")
	}
}

// The rule builders are the last mile, and they used to parse ports the loose
// way the validation upstream was fixed not to: fmt.Sscanf stops at the first
// character it cannot read and reports success for the rest.
func TestParsePort_RejectsTrailingRubbish(t *testing.T) {
	cases := []struct {
		in   string
		want uint16
	}{
		{"22", 22},
		{"65535", 65535},
		{" 443 ", 443},
		{"80abc", 0},
		{"80 90", 0},
		{"0", 0},
		{"65536", 0},
		{"-1", 0},
		{"", 0},
		{"http", 0},
	}
	for _, tc := range cases {
		if got := parsePort(tc.in); got != tc.want {
			t.Errorf("parsePort(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestBuildPortExprs_RangeAndSingle(t *testing.T) {
	// A range produces two bounds; a single port one equality.
	if n := len(buildPortExprs("8000:9000")); n != 3 {
		t.Errorf("a range needs a payload load and two bounds, got %d expressions", n)
	}
	if n := len(buildPortExprs("443")); n != 2 {
		t.Errorf("a single port needs a payload load and one comparison, got %d expressions", n)
	}
	// A malformed range must not match whichever half happened to parse.
	for _, bad := range []string{"8000:abc", "abc:9000", "9000:8000", "8000:9000junk"} {
		exprs := buildPortExprs(bad)
		if len(exprs) != 2 {
			t.Errorf("buildPortExprs(%q) produced a range match from an invalid range", bad)
		}
	}
}

// Snapshot rotation is pointed at log_dir, which also holds audit.log — and it
// used to take every non-directory file in there. "audit.log" sorts before
// "nftables_…", so it was the first thing removed: on the eleventh apply,
// easywall deleted the security record that audit-log.md calls append-only and
// never truncated by easywall.
func TestRotateSnapshots_TouchesNothingItDidNotWrite(t *testing.T) {
	dir := t.TempDir()

	bystanders := []string{"audit.log", "audit.log.1", "audit.log.2.gz", "README"}
	for _, name := range bystanders {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("keep me"), 0640); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("nftables_2026-08-09_16-00-%02d.000.json", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	if err := SaveSnapshot(dir, []byte(`{"tables":[]}`)); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	for _, name := range bystanders {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("rotation deleted %q, which it did not write: %v", name, err)
		}
	}
}

func TestRotateSnapshots_KeepsTheNewestTen(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 15; i++ {
		name := fmt.Sprintf("nftables_2026-08-09_16-00-%02d.000.json", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	if err := SaveSnapshot(dir, []byte(`{"tables":[]}`)); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != snapshotsKept {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected %d snapshots, got %d: %v", snapshotsKept, len(entries), names)
	}
	// The oldest must be the ones that went.
	for _, e := range entries {
		if strings.Contains(e.Name(), "16-00-00") || strings.Contains(e.Name(), "16-00-05") {
			t.Errorf("an older snapshot survived while newer ones were kept: %s", e.Name())
		}
	}
}

// Two applies inside the same second must not land on the same filename.
func TestSaveSnapshot_NamesAreDistinctWithinASecond(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 3; i++ {
		if err := SaveSnapshot(dir, []byte(`{"tables":[]}`)); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Errorf("three snapshots in quick succession must not overwrite each other, got %d", len(entries))
	}
}
