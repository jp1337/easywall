package core

import (
	"path/filepath"
	"testing"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"github.com/jp1337/easywall/internal/shared"
)

// recordingConn captures the rules a builder adds without a kernel. It exists so
// the expression-level guards in this package run under `make test` rather than
// only under -tags integration, where they need root.
//
// AddRule returns what it was given, which is what *nftables.Conn does: the
// builders in nftables.go discard the return value, and a recorder that returned
// nil would still satisfy them — but a future builder that reads it back would
// find a nil in a test and a rule in production, which is the worst shape a
// double is allowed to have.
type recordingConn struct{ rules []*nftables.Rule }

func (r *recordingConn) AddRule(rule *nftables.Rule) *nftables.Rule {
	r.rules = append(r.rules, rule)
	return rule
}

// The table and chain are only carried through to the kernel rule; nothing in
// the builders reads a field of either, so a named pair is enough to prove the
// rule was addressed at the input chain of the easywall table rather than
// somewhere else.
func easywallInetTableForTest() *nftables.Table {
	return &nftables.Table{Name: tableName, Family: nftables.TableFamilyINet}
}

func inputChainForTest(t *nftables.Table) *nftables.Chain {
	return &nftables.Chain{Name: inputChainName, Table: t}
}

// The established-accept rule is the one rule whose counter is a health signal,
// and it was invisible: it carried no counter and no comment, so RuleCounters —
// which skips every rule without an id — never saw it.
func TestEstablishedRuleIsTaggedAndCounted(t *testing.T) {
	tbl := easywallInetTableForTest()
	ch := inputChainForTest(tbl)

	rec := &recordingConn{}
	m := &NftablesManager{adder: rec}

	m.addEstablishedAccept(tbl, ch)

	if len(rec.rules) != 1 {
		t.Fatalf("addEstablishedAccept added %d rules, want 1", len(rec.rules))
	}
	r := rec.rules[0]

	if r.Table != tbl || r.Chain != ch {
		t.Errorf("the rule was addressed at table %v chain %v, want the easywall input chain",
			r.Table, r.Chain)
	}

	id, ok := idFromUserData(r.UserData)
	if !ok || id != ReservedIDEstablished {
		t.Errorf("UserData id = %q (ok=%v), want %q", id, ok, ReservedIDEstablished)
	}

	var counters int
	for _, e := range r.Exprs {
		if _, is := e.(*expr.Counter); is {
			counters++
		}
	}
	if counters != 1 {
		t.Errorf("the rule carries %d counters, want exactly 1", counters)
	}

	// After the match, before the verdict — the position portAcceptRules
	// documents at length and for the same reason. A counter ahead of the ct
	// state match counts every packet that reaches the rule rather than every
	// packet it accepts, which on this rule would turn the one number health
	// reads into a figure that rises on a host whose stateful half matches
	// nothing at all.
	last := len(r.Exprs) - 1
	if last < 1 {
		t.Fatalf("the rule carries %d expressions", len(r.Exprs))
	}
	if _, is := r.Exprs[last].(*expr.Verdict); !is {
		t.Errorf("the last expression is %T, want the verdict", r.Exprs[last])
	}
	if _, is := r.Exprs[last-1].(*expr.Counter); !is {
		t.Errorf("the expression before the verdict is %T, want the counter", r.Exprs[last-1])
	}

	// The forward chain gets the same rule and must not get the same id.
	// addForwardExceptions adds it there so a reply is not re-tested against the
	// routed networks, and one id on two rules is a figure that sums two chains
	// under one name the moment anything reads counters more widely than
	// RuleCounters does. Untagged is not an oversight — see addEstablishedAccept.
	fwd := &recordingConn{}
	(&NftablesManager{adder: fwd}).addEstablishedAccept(tbl, &nftables.Chain{Name: "forward", Table: tbl})
	if len(fwd.rules) != 1 {
		t.Fatalf("the forward copy added %d rules, want 1", len(fwd.rules))
	}
	if id, ok := idFromUserData(fwd.rules[0].UserData); ok {
		t.Errorf("the forward chain's copy is tagged %q; the reserved id has to name one "+
			"rule, or a reader that does not filter by chain sums two of them", id)
	}
	var fwdCounters int
	for _, e := range fwd.rules[0].Exprs {
		if _, is := e.(*expr.Counter); is {
			fwdCounters++
		}
	}
	if fwdCounters != 1 {
		t.Errorf("the forward copy carries %d counters, want 1 — the counter is on both "+
			"copies, only the id is not", fwdCounters)
	}
}

// A reserved id must never be booked into usage.json. UsageStore.Collect skips
// ids that are not live and liveRuleIDs only knows rules from the rules file, so
// the pruning already drops it — but the coupling is invisible, and a change to
// either side would make _established a phantom port in Last used.
func TestIsReservedRuleID(t *testing.T) {
	for _, id := range []string{"_established", "_anything"} {
		if !IsReservedRuleID(id) {
			t.Errorf("IsReservedRuleID(%q) = false, want true", id)
		}
	}
	for _, id := range []string{"a1b2c3d4e5f6", "", "established"} {
		if IsReservedRuleID(id) {
			t.Errorf("IsReservedRuleID(%q) = true, want false", id)
		}
	}
}

// The prefix has to hold on its own, not because liveRuleIDs happens not to
// name the reserved id. The mutation that proves this test is load-bearing is
// dropping IsReservedRuleID from Collect's prune loop — the one place the
// prefix is checked: _established is then booked as a port rule and Last used
// gains a phantom entry that no rules file can ever delete.
func TestReservedRuleIDsNeverReachUsage(t *testing.T) {
	dir := t.TempDir()
	u := NewUsageStore(filepath.Join(dir, "usage.json"))

	counters := map[string]RuleCounter{
		ReservedIDEstablished: {Packets: 500, Bytes: 40000},
		"tcp-22-a1b2":         {Packets: 3, Bytes: 180},
	}
	// The reserved id is deliberately marked live, which is the state a future
	// liveRuleIDs bug would produce. The prefix must hold on its own.
	live := map[string]bool{ReservedIDEstablished: true, "tcp-22-a1b2": true}

	if err := u.Collect(counters, live); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	res, err := u.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, present := res.Usage[ReservedIDEstablished]; present {
		t.Errorf("usage.json holds %q; a reserved id is not a port rule",
			ReservedIDEstablished)
	}
	if got := res.Usage["tcp-22-a1b2"].Packets; got != 3 {
		t.Errorf("the real rule booked %d packets, want 3", got)
	}
}

// A record written before the prefix existed heals rather than lingering: the
// prune loop drops it on the next collect, whatever liveness says about it.
func TestReservedRuleIDsWrittenByAnOlderVersionArePruned(t *testing.T) {
	dir := t.TempDir()
	u := NewUsageStore(filepath.Join(dir, "usage.json"))

	// Seeded through the store's own writer rather than by hand, so the file is
	// exactly the shape the older version left behind.
	if err := u.write(shared.UsageResult{Usage: map[string]shared.RuleUsage{
		ReservedIDEstablished: {Packets: 500, Bytes: 40000, KernelPackets: 500, KernelBytes: 40000},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := u.Collect(map[string]RuleCounter{}, map[string]bool{ReservedIDEstablished: true}); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	res, err := u.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, present := res.Usage[ReservedIDEstablished]; present {
		t.Errorf("usage.json still holds %q after a collect; the record does not heal",
			ReservedIDEstablished)
	}
}
