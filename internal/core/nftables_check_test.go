package core

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"

	"github.com/jp1337/easywall/internal/shared"
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

// A second conntrack load must not carry the mask out of the check's scope.
//
// precededByCtState walks back rather than reading i-1 precisely so that a
// counter or a payload test growing between the load and the mask does not
// silently drop the rule out of scope. Returning at the first expr.Ct of *any*
// key defeated that: this rule's Ct{COUNT} came between, and the byte-reversed
// mask behind it was reported clean.
//
// The second case is the price of the fix, and is why the loop stops at the
// nearest load into the register the mask reads rather than at the nearest
// state load anywhere. A count written into the register the mask goes on to
// read is not a ct-state comparison at all, and reporting it as a broken one
// would be a false alarm in a gate that fails the build.
//
// Both fixtures carry the *byte-reversed* mask, and the second one has to.
// Written with a native mask it produced no finding under either
// implementation — the mask is correct, so there is nothing to report whatever
// precededByCtState answers — and the assertion could not see the false
// positive it exists to prevent: the whole test passed against the bare
// `continue`. That made this the eleventh guard in this release green for the
// wrong reason, and the third turn of the same screw: the original defect was
// a unit test recording the reversed mask as expected output, Task 10's
// substring-guard helper was itself a substring guard, and this was the test
// written to pin a fix for a guard green for the wrong reason, blind to the
// thing it was written for. Reversing the mask is the whole difference: the
// register scoping is now the only reason the second case reports nothing.
func TestCheckSeesPastASecondConntrackLoad(t *testing.T) {
	rule := func(second *expr.Ct, mask []byte) *nftables.Rule {
		return &nftables.Rule{
			Chain: &nftables.Chain{Name: "input"},
			Exprs: []expr.Any{
				&expr.Ct{Register: 1, Key: expr.CtKeySTATE},
				second,
				&expr.Counter{},
				&expr.Bitwise{
					SourceRegister: 1, DestRegister: 1, Len: 4,
					Mask: mask,
					Xor:  ctStateNoMatch,
				},
				&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: ctStateNoMatch},
				&expr.Verdict{Kind: expr.VerdictAccept},
			},
		}
	}

	reversed := binaryutil.BigEndian.PutUint32(ctStateEstablished | ctStateRelated)

	// A count loaded into another register leaves register 1 holding the state,
	// so the reversed mask is still this check's business.
	if findings := CheckRules([]*nftables.Rule{
		rule(&expr.Ct{Register: 2, Key: expr.CtKeyPKTS}, reversed)}, nil); len(findings) != 1 {
		t.Errorf("findings = %d, want 1: a conntrack load into another register left a "+
			"byte-reversed ct-state mask unchecked — the walk-back is defeated by the "+
			"very expression it was written to see past", len(findings))
	}

	// The same load into register 1 overwrites the state, so the mask is not a
	// ct-state comparison and must not be judged as one — even though those four
	// bytes would be a defect if it were.
	if findings := CheckRules([]*nftables.Rule{
		rule(&expr.Ct{Register: 1, Key: expr.CtKeyPKTS}, reversed)}, nil); len(findings) != 0 {
		t.Errorf("findings = %+v, want none: the mask reads a packet count, not a "+
			"conntrack state, and a false alarm in a gate that fails the build is how "+
			"the gate gets switched off", findings)
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

// The position in a finding has to be the position of the rule it is about.
// Both per-chain checks reported a plain zero, so a jump at rule 3 was written
// into the audit log as "input rule 0" — a number that sends whoever reads it
// to the wrong rule, which is worse than no number at all. The chain that
// accepts genuinely has no one position, and prints none.
func TestCheckReportsTheRulePositionItIsAbout(t *testing.T) {
	filler := func() *nftables.Rule {
		return &nftables.Rule{Chain: &nftables.Chain{Name: "input"}, Exprs: []expr.Any{
			&expr.Verdict{Kind: expr.VerdictDrop}}}
	}
	rules := []*nftables.Rule{
		filler(), filler(), filler(),
		{Chain: &nftables.Chain{Name: "input"}, Exprs: []expr.Any{
			&expr.Verdict{Kind: expr.VerdictJump, Chain: "gone"}}},
	}
	findings := CheckRules(rules, nil)
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1: %+v", len(findings), findings)
	}
	if findings[0].Index != 3 {
		t.Errorf("Index = %d, want 3", findings[0].Index)
	}
	if want := "input rule 3: "; !strings.HasPrefix(findings[0].String(), want) {
		t.Errorf("String() = %q, want it to start %q", findings[0].String(), want)
	}

	// And the chain-level finding names no position rather than a false zero.
	accepting := []*nftables.Rule{
		filler(),
		{Chain: &nftables.Chain{Name: "input"}, Exprs: []expr.Any{
			&expr.Verdict{Kind: expr.VerdictJump, Chain: "sshbrute"}}},
		{Chain: &nftables.Chain{Name: "sshbrute"}, Exprs: []expr.Any{
			&expr.Verdict{Kind: expr.VerdictAccept}}},
	}
	f := CheckRules(accepting, nil)
	if len(f) != 1 {
		t.Fatalf("findings = %d, want 1: %+v", len(f), f)
	}
	// On the prefix, not Contains: the Reason itself says "every rule after the
	// jump", so a Contains test would pass whatever the position printed as.
	if want := "sshbrute: a jumped-to chain"; !strings.HasPrefix(f[0].String(), want) {
		t.Errorf("String() = %q, want it to start %q — no position on a "+
			"chain-level finding", f[0].String(), want)
	}
	if f[0].Index != noIndex {
		t.Errorf("Index = %d, want noIndex (%d)", f[0].Index, noIndex)
	}
}

// Two chains jumping to the same missing target are two wrong rules in two
// different builders. The jump sites were held in a map to a single chain name,
// so one of them was reported and the other was not — and which one survived
// depended on the order they were added in.
func TestCheckNamesEveryJumperToAMissingChain(t *testing.T) {
	rules := []*nftables.Rule{
		{Chain: &nftables.Chain{Name: "input"}, Exprs: []expr.Any{
			&expr.Verdict{Kind: expr.VerdictJump, Chain: "gone"}}},
		{Chain: &nftables.Chain{Name: "forward"}, Exprs: []expr.Any{
			&expr.Verdict{Kind: expr.VerdictGoto, Chain: "gone"}}},
	}
	findings := CheckRules(rules, nil)
	if len(findings) != 2 {
		t.Fatalf("findings = %d, want 2 — one per jumping rule: %+v", len(findings), findings)
	}
	named := map[string]bool{}
	for _, f := range findings {
		named[f.Chain] = true
	}
	if !named["input"] || !named["forward"] {
		t.Errorf("findings named %v, want both input and forward", named)
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

// A layer-B finding reaches the audit log, and nothing does when there is none.
//
// The finding used to exist in two places only: the journal, and a health reply
// nobody had asked for. The audit log is the record an operator greps after an
// incident, and it said the apply had simply succeeded.
func TestAuditBuildFindings(t *testing.T) {
	one := Finding{Chain: "input", Index: 3, Reason: "a ct state mask that matches nothing"}
	two := Finding{Chain: "ssh-brute", Index: noIndex, Reason: "a jumped-to chain ends in accept"}

	cases := []struct {
		name       string
		findings   []Finding
		wantWrite  bool
		wantDetail string
	}{
		{name: "no findings", findings: nil, wantWrite: false},
		{name: "empty slice", findings: []Finding{}, wantWrite: false},
		{
			name: "one finding is recorded verbatim", findings: []Finding{one},
			wantWrite:  true,
			wantDetail: "input rule 3: a ct state mask that matches nothing",
		},
		{
			// Not all of them: a malformed ruleset can produce one per rule, and
			// this text is rendered on a web page. The journal has the rest.
			name: "more than one is counted", findings: []Finding{one, two, one},
			wantWrite:  true,
			wantDetail: "input rule 3: a ct state mask that matches nothing (and 2 more; see the journal)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "audit.log")
			auditBuildFindings(path, tc.findings, "web")

			raw, err := os.ReadFile(path)
			if !tc.wantWrite {
				if err == nil {
					t.Fatalf("an apply with no findings wrote %q; a clean apply must not "+
						"report itself degraded", string(raw))
				}
				return
			}
			if err != nil {
				t.Fatalf("nothing was written for %d finding(s): %v", len(tc.findings), err)
			}

			var entry shared.AuditLogEntry
			if err := json.Unmarshal([]byte(strings.TrimSpace(string(raw))), &entry); err != nil {
				t.Fatalf("audit line %q: %v", string(raw), err)
			}
			if entry.Action != "health_degraded" {
				t.Errorf("audit action = %q, want %q", entry.Action, "health_degraded")
			}
			if entry.Detail != tc.wantDetail {
				t.Errorf("audit detail =\n  %q\nwant\n  %q", entry.Detail, tc.wantDetail)
			}
			if entry.User != "web" {
				t.Errorf("audit user = %q; the finding belongs to whoever ran the apply", entry.User)
			}
		})
	}
}

// The recording adder is the only door to the connection.
//
// CheckRules reads m.built, and only builtRecorder.AddRule fills it. All
// thirty-one existing builders go through m.adder — and nothing stopped a
// thirty-second from calling m.conn.AddRule directly, which is what every
// builder in this file looked like before c4dab40, so it is the shape a
// copy-paste out of git history produces.
//
// The review measured what that costs. A 2.18-shaped builder written the
// pre-2.17 way, carrying binaryutil.BigEndian.PutUint32(ctStateNew) — the mask
// class of the original defect — left `go test ./internal/...` green, the
// integration gate green, m.LastFindings() empty, and `ct state 0x8000000` in
// the live kernel table. All three layers reported success, and the gate's own
// three defences survived it intact: checksRun was still 2, the len(m.built)
// floor was met by the other rules, and the findings loop never saw the rule.
// A rule that leaves m.built leaves every layer of this release.
//
// The AST and not a substring, and this file's own subject is why:
// nftables.go carries the comment "Thirty-one builders called m.conn.AddRule
// directly", so a strings.Contains guard would match the sentence describing
// the thing it is checking and pass. Six of the nine guards this release found
// green for the wrong reason failed exactly that way.
//
// Every non-test source in the package, not nftables.go alone. A guard whose
// scope is one filename is one commit away from being wrong about its own
// subject — the same reasoning coreSources carries, which was widened after
// naming firewall.go and restore.go made it blind to a third writer of the
// table. A builder in a sibling file is exactly as invisible to layer B.
//
// What it still does not see is carried in carried-forward.md: `cn := m.conn`
// followed by `cn.AddRule(…)` has no `.conn.AddRule` selector left to match,
// and refusing it needs type resolution rather than syntax.
func TestEveryRuleIsAddedThroughTheRecordingAdder(t *testing.T) {
	recorders := 0
	for file, src := range coreSources(t) {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			// builtRecorder.AddRule is the one place that may reach the
			// connection: it is the forwarding half of the recorder, and it has
			// already appended to m.built by the time it gets there.
			if fn.Name.Name == "AddRule" && receiverTypeName(fn) == "builtRecorder" {
				recorders++
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				// A selector rather than a call, so a method value —
				// `add := m.conn.AddRule` — is refused as well as a call.
				sel, ok := n.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "AddRule" {
					return true
				}
				inner, ok := sel.X.(*ast.SelectorExpr)
				if !ok || inner.Sel.Name != "conn" {
					return true
				}
				t.Errorf("%s:%d: .conn.AddRule in %s; every rule must be added through "+
					"m.adder, because CheckRules reads m.built and only "+
					"builtRecorder.AddRule fills it — a rule added straight to the "+
					"connection reaches the kernel unchecked by layer B, and invisible "+
					"to the integration gate and to LastFindings",
					file, fset.Position(sel.Pos()).Line, fn.Name.Name)
				return true
			})
		}
	}

	// Not finding builtRecorder.AddRule means this walk skipped nothing across
	// the whole package, which means it is reading a corpus that no longer has
	// the recorder in it — the one way a guard built around an exemption passes
	// by inspecting nothing. coreSources has its own floor on the file count.
	if recorders != 1 {
		t.Fatalf("found %d builtRecorder.AddRule declarations in internal/core, want 1: "+
			"this guard is inspecting the wrong thing", recorders)
	}
}

// receiverTypeName is the receiver's type name, pointer or not, and "" for a
// function that has no receiver.
func receiverTypeName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	t := fn.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
