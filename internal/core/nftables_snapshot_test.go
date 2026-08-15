//go:build integration

package core

import (
	"encoding/json"
	"os/exec"
	"testing"
)

// snapshotDoc is the shape Snapshot writes, for reading it back.
type snapshotDoc struct {
	Timestamp string `json:"timestamp"`
	Tables    []struct {
		Name   string `json:"name"`
		Family string `json:"family"`
		Error  string `json:"error,omitempty"`
		Chains []struct {
			Name  string `json:"name"`
			Rules *int   `json:"rules"`
			Error string `json:"error,omitempty"`
		} `json:"chains"`
	} `json:"tables"`
}

// A chain belongs to a table by name *and* family. Snapshot matched on the name
// alone, so every table was credited with the chains of every same-named table
// in another family, and the counts beside them were read from the wrong table.
//
// The collision is not exotic — `table ip easywall` is what a hand-written
// ruleset alongside easywall looks like, and nft allows it. Measured before the
// fix, with ip easywall holding chains input and decoy and inet easywall holding
// input:
//
//	ip   easywall: input(1), decoy(1), input(1)   ← two chains reported as three
//	inet easywall: input(1), decoy(0), input(1)   ← one chain reported as three
//
// The `decoy(0)` in the inet table is the worse half: GetRules failed and the
// count stayed at its zero value, so a chain that does not exist was reported as
// one that exists and is empty. This file is written to log_dir on every apply
// and is what an operator opens after a lockout.
func TestSnapshotAttributesEachChainToItsOwnFamily(t *testing.T) {
	nft := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("nft", args...).CombinedOutput(); err != nil {
			t.Fatalf("nft %v: %v: %s", args, err, out)
		}
	}

	m, err := NewNftablesManager()
	if err != nil {
		t.Fatal(err)
	}

	nft("add", "table", "ip", tableName)
	defer func() { _ = exec.Command("nft", "delete", "table", "ip", tableName).Run() }()
	nft("add", "chain", "ip", tableName, "input",
		"{ type filter hook input priority 0 ; policy accept ; }")
	nft("add", "rule", "ip", tableName, "input", "tcp dport 9999 accept")
	nft("add", "chain", "ip", tableName, "decoy")
	nft("add", "rule", "ip", tableName, "decoy", "accept")

	nft("add", "table", "inet", tableName)
	defer func() { _ = exec.Command("nft", "delete", "table", "inet", tableName).Run() }()
	nft("add", "chain", "inet", tableName, "input",
		"{ type filter hook input priority 0 ; policy drop ; }")
	nft("add", "rule", "inet", tableName, "input", "tcp dport 22 accept")

	raw, err := m.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	var doc snapshotDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("snapshot does not parse: %v (%s)", err, raw)
	}

	want := map[string]map[string]int{
		"ip":   {"input": 1, "decoy": 1},
		"inet": {"input": 1},
	}

	seen := 0
	for _, tbl := range doc.Tables {
		if tbl.Name != tableName {
			continue
		}
		expected, ok := want[tbl.Family]
		if !ok {
			t.Errorf("snapshot holds a %q table in family %q, which the test did not create",
				tableName, tbl.Family)
			continue
		}
		seen++
		if len(tbl.Chains) != len(expected) {
			t.Errorf("%s %s: %d chains, want %d — %s",
				tbl.Family, tbl.Name, len(tbl.Chains), len(expected), raw)
		}
		for _, ch := range tbl.Chains {
			n, ok := expected[ch.Name]
			if !ok {
				t.Errorf("%s %s: chain %q does not exist in that table", tbl.Family, tbl.Name, ch.Name)
				continue
			}
			// A count that could not be read must not be reported as zero.
			if ch.Rules == nil {
				t.Errorf("%s %s %s: rule count unread (%s)", tbl.Family, tbl.Name, ch.Name, ch.Error)
				continue
			}
			if *ch.Rules != n {
				t.Errorf("%s %s %s: %d rules, want %d", tbl.Family, tbl.Name, ch.Name, *ch.Rules, n)
			}
		}
	}
	if seen != 2 {
		t.Errorf("expected both %q tables in the snapshot, saw %d: %s", tableName, seen, raw)
	}
}

// Enforcing asks about `table inet easywall` and must not be swayed by a
// same-named table in another family. It was already right — the chain object is
// used only for its name, and GetRules queries by the table's family — but
// nothing said so, and the loop it depends on has no family test in it.
func TestEnforcingIgnoresASameNamedTableInAnotherFamily(t *testing.T) {
	nft := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("nft", args...).CombinedOutput(); err != nil {
			t.Fatalf("nft %v: %v: %s", args, err, out)
		}
	}

	m, err := NewNftablesManager()
	if err != nil {
		t.Fatal(err)
	}

	nft("add", "table", "ip", tableName)
	defer func() { _ = exec.Command("nft", "delete", "table", "ip", tableName).Run() }()
	nft("add", "chain", "ip", tableName, "input",
		"{ type filter hook input priority 0 ; policy accept ; }")
	nft("add", "rule", "ip", tableName, "input", "tcp dport 9999 accept")

	if m.Enforcing() {
		t.Error("Enforcing() is true with only `table ip easywall` present; " +
			"easywall owns the inet table and nothing is being enforced")
	}

	nft("add", "table", "inet", tableName)
	defer func() { _ = exec.Command("nft", "delete", "table", "inet", tableName).Run() }()
	nft("add", "chain", "inet", tableName, "input",
		"{ type filter hook input priority 0 ; policy drop ; }")
	nft("add", "rule", "inet", tableName, "input", "tcp dport 22 accept")

	if !m.Enforcing() {
		t.Error("Enforcing() is false with a populated inet input chain")
	}

	// The case that matters: easywall's own chain emptied, the decoy still full.
	nft("flush", "chain", "inet", tableName, "input")
	if m.Enforcing() {
		t.Error("Enforcing() is true with an empty inet input chain — " +
			"the dashboard would report live rules after the table was flushed")
	}
}
