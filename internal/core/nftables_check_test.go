package core

import (
	"strings"
	"testing"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
)

// The regression test for the defect this release exists for. All three ct-state
// masks in nftables.go were written big-endian; the kernel compares a native
// u32, so `ct state established,related accept` matched no packet for five
// releases and the rule reported itself present the whole time.
func TestCheckRejectsAByteReversedCtStateMask(t *testing.T) {
	reversed := &nftables.Rule{
		Chain: &nftables.Chain{Name: "input"},
		Exprs: []expr.Any{
			&expr.Ct{Register: 1, Key: expr.CtKeySTATE},
			&expr.Bitwise{
				SourceRegister: 1, DestRegister: 1, Len: 4,
				Mask: binaryutil.BigEndian.PutUint32(ctStateEstablished | ctStateRelated),
				Xor:  ctStateNoMatch,
			},
			&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: ctStateNoMatch},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	}

	findings := CheckRules([]*nftables.Rule{reversed}, nil)
	if len(findings) != 1 {
		t.Fatalf("CheckRules returned %d findings, want 1: %+v", len(findings), findings)
	}
	if !strings.Contains(findings[0].Reason, "conntrack state") {
		t.Errorf("Reason = %q, want it to name the conntrack state", findings[0].Reason)
	}
}

// The same rule built the way nftables.go builds it now must pass, or the check
// is a permanent alarm and gets switched off by whoever meets it.
func TestCheckAcceptsTheNativeCtStateMask(t *testing.T) {
	m := &NftablesManager{}
	rec := &recordingConn{}
	m.adder = rec
	m.addEstablishedAccept(
		&nftables.Table{Name: "easywall"}, &nftables.Chain{Name: "input"})

	if findings := CheckRules(rec.rules, nil); len(findings) != 0 {
		t.Errorf("the shipped rule produced findings: %+v", findings)
	}
}

// A mask naming a bit no conntrack state uses is the same class of defect
// arriving from the other direction: a typo in the constant rather than in the
// byte order.
func TestCheckRejectsACtStateBitTheKernelDoesNotDefine(t *testing.T) {
	r := &nftables.Rule{
		Chain: &nftables.Chain{Name: "input"},
		Exprs: []expr.Any{
			&expr.Ct{Register: 1, Key: expr.CtKeySTATE},
			&expr.Bitwise{
				SourceRegister: 1, DestRegister: 1, Len: 4,
				Mask: binaryutil.NativeEndian.PutUint32(0x8000000),
				Xor:  ctStateNoMatch,
			},
			&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: ctStateNoMatch},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	}
	if findings := CheckRules([]*nftables.Rule{r}, nil); len(findings) != 1 {
		t.Fatalf("findings = %d, want 1 for mask 0x8000000", len(findings))
	}
}

// A bitwise that does not follow a conntrack load is an address mask or a TCP
// flags mask, and this check has nothing to say about it. Without the
// precedence test every whitelisted network in the table would be reported: an
// IPv4 netmask of /8 is 0xff000000, which names no conntrack state at all.
func TestCheckIgnoresABitwiseThatIsNotACtStateMask(t *testing.T) {
	r := &nftables.Rule{
		Chain: &nftables.Chain{Name: "input"},
		Exprs: []expr.Any{
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       12,
				Len:          4,
			},
			&expr.Bitwise{
				SourceRegister: 1, DestRegister: 1, Len: 4,
				Mask: []byte{0xff, 0x00, 0x00, 0x00},
				Xor:  []byte{0, 0, 0, 0},
			},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{10, 0, 0, 0}},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	}
	if findings := CheckRules([]*nftables.Rule{r}, nil); len(findings) != 0 {
		t.Errorf("a /8 source-address mask was reported: %+v", findings)
	}
}

// The bug in reach_integration_test.go:184: the SSH brute-force chain ended in
// accept and was jumped to before the blacklist, so a blacklisted address could
// open SSH as long as it stayed under the rate limit. A jumped-to chain must
// return or drop, and the exceptions are named in one list a reviewer sees.
func TestCheckRejectsAJumpedChainThatAccepts(t *testing.T) {
	rules := []*nftables.Rule{
		{Chain: &nftables.Chain{Name: "input"}, Exprs: []expr.Any{
			&expr.Verdict{Kind: expr.VerdictJump, Chain: "sshbrute"}}},
		{Chain: &nftables.Chain{Name: "sshbrute"}, Exprs: []expr.Any{
			&expr.Verdict{Kind: expr.VerdictAccept}}},
	}
	findings := CheckRules(rules, nil)
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1: %+v", len(findings), findings)
	}
	if !strings.Contains(findings[0].Reason, "accept") {
		t.Errorf("Reason = %q, want it to name the accept", findings[0].Reason)
	}
	// And it passes once the chain is declared accepting.
	if f := CheckRules(rules, map[string]bool{"sshbrute": true}); len(f) != 0 {
		t.Errorf("a declared accepting chain still produced findings: %+v", f)
	}
}

// A jump to a chain the same transaction does not create is a rule the kernel
// rejects at flush time with an error that names a number, not a chain.
func TestCheckRejectsAJumpToAChainNobodyCreates(t *testing.T) {
	rules := []*nftables.Rule{
		{Chain: &nftables.Chain{Name: "input"}, Exprs: []expr.Any{
			&expr.Verdict{Kind: expr.VerdictJump, Chain: "portscan"}}},
	}
	if findings := CheckRules(rules, nil); len(findings) != 1 {
		t.Fatalf("findings = %d, want 1 for a jump to an absent chain", len(findings))
	}
}

// Every kind expr.VerdictKind defines must pass. The switch that classifies them
// listed eight of the eleven while this release was being planned, which would
// have reported `stolen`, `repeat` and `stop` as verdicts the kernel does not
// define — a false positive in a gate that fails the build, and a gate that
// cries wolf is a gate somebody switches off.
func TestCheckKnowsEveryVerdictKindTheLibraryDefines(t *testing.T) {
	known := []expr.VerdictKind{
		expr.VerdictReturn, expr.VerdictGoto, expr.VerdictJump, expr.VerdictBreak,
		expr.VerdictContinue, expr.VerdictDrop, expr.VerdictAccept,
		expr.VerdictStolen, expr.VerdictQueue, expr.VerdictRepeat, expr.VerdictStop,
	}
	for _, kind := range known {
		// A jump and a goto name a chain, and a target nothing creates is a
		// different finding than the one this test is about — so point both at
		// the chain the rule is already in.
		rules := []*nftables.Rule{
			{Chain: &nftables.Chain{Name: "input"}, Exprs: []expr.Any{
				&expr.Verdict{Kind: kind, Chain: "input"}}},
		}
		if f := CheckRules(rules, nil); len(f) != 0 {
			t.Errorf("verdict kind %d was reported as unknown: %+v", kind, f)
		}
	}

	// And a value outside the enum still is. VerdictStop is 5, so 6 is the
	// first number no kernel verdict occupies.
	rules := []*nftables.Rule{
		{Chain: &nftables.Chain{Name: "input"}, Exprs: []expr.Any{
			&expr.Verdict{Kind: expr.VerdictKind(6)}}},
	}
	if f := CheckRules(rules, nil); len(f) != 1 {
		t.Fatalf("findings = %d, want 1 for verdict kind 6: %+v", len(f), f)
	}
}
