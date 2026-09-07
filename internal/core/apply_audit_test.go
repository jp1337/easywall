package core

import (
	"strings"
	"testing"
)

// Every way out of apply leaves a line in the audit log.
//
// The audit log is what an operator opens when the interface says the firewall
// is off. 2.7 gave the first GetState an entry and left its four neighbours —
// BackupCurrent, PromoteStaged, the re-read GetState and acceptance.Start — each
// returning an error into a journal nobody is reading, on a machine whose rules
// did not change and whose interface says nothing about why.
//
// Matched by each branch's own audit message text, not by byte proximity. A
// single distance window cannot serve every branch here: the kernel-write
// branch carries the panic-marker check and its comment between the audit
// write and the return, so a window wide enough to reach past that is also
// wide enough to reach backward from the "backup" return across its own audit
// entry to GetState's, a few lines above — which would pass with backup's
// entry deleted. Each branch is checked against the text nothing else in the
// function repeats, in the span between it and the previous of these five
// returns, so a deleted entry cannot be covered by a neighbour's.
func TestEveryFailurePathInApplyIsAudited(t *testing.T) {
	body := funcBody(t, coreSource(t, "firewall.go"), "firewall.go", "func (f *Firewall) apply(")

	returns := indexesOf(body, "return fmt.Errorf(")
	if len(returns) < 5 {
		t.Fatalf("apply has %d error returns; this guard was written against five "+
			"(get rules, backup, promote, re-read, kernel write) and has to be updated "+
			"together with the function", len(returns))
	}

	// One entry per return, in source order, naming the text that only its own
	// audit call contains. "get rules" and "kernel write" share the same
	// generic WriteAuditLog(...err.Error(), user) shape, so they are told apart
	// by which segment (bounded by the previous return in this list) contains
	// the match, not by the text alone.
	cases := []struct {
		name        string
		returnMatch string
		auditText   string
	}{
		{"get rules", `return fmt.Errorf("get rules: %w", err)`,
			`WriteAuditLog(f.cfg.AuditLogPath(), "apply_failed", "all", err.Error(), user)`},
		{"backup", `return fmt.Errorf("backup rules: %w", err)`,
			"the current rules could not be backed up"},
		{"promote", `return fmt.Errorf("promote staged rules: %w", err)`,
			"the staged rules could not be promoted"},
		{"re-read", `return fmt.Errorf("re-read rules after promote: %w", err)`,
			"the rules could not be re-read after promoting"},
		// acceptance.Start's error path returns a bare "return err", not
		// "return fmt.Errorf(...)", so it is not one of the five counted above —
		// but it is the fourth audit-silent path this task closes, and its
		// rollback has a test of its own below. Checked here too, in source
		// order between "re-read" and "kernel write", so a deleted audit entry
		// on this path is caught the same way as the other three.
		{"acceptance.Start", "return err", "the acceptance window could not be opened"},
		{"kernel write", `return fmt.Errorf("apply nftables rules: %w", err)`,
			`WriteAuditLog(f.cfg.AuditLogPath(), "apply_failed", "all", err.Error(), user)`},
	}

	segmentStart := 0
	for _, c := range cases {
		at := strings.Index(body[segmentStart:], c.returnMatch)
		if at < 0 {
			t.Fatalf("%s: return statement %q not found after byte %d; apply's shape has "+
				"changed and this guard needs updating with it", c.name, c.returnMatch, segmentStart)
		}
		at += segmentStart
		segment := body[segmentStart:at]
		if !strings.Contains(segment, c.auditText) {
			t.Errorf("%s: no audit entry containing %q between the previous checked return "+
				"and this one; the operator sees an unchanged firewall and finds "+
				"nothing in the log saying why", c.name, c.auditText)
		}
		segmentStart = at + len(c.returnMatch)
	}
}

// And the window that could not be opened does not leave Current holding a set
// nobody confirmed.
//
// PromoteStaged has already run by the time acceptance.Start is called, so a
// Start that fails leaves the *stored* Current equal to the set this apply was
// trying out — with no window, no kernel write and no rollback. A reboot or a
// resume then installs it with no acceptance window at all, because
// RestoreCurrent's whole justification is that Current has already survived one.
// It has not. That is the 2.7 lockout chain exactly, reached by a different door.
func TestApply_AFailedAcceptanceStartRollsTheFileBack(t *testing.T) {
	body := funcBody(t, coreSource(t, "firewall.go"), "firewall.go", "func (f *Firewall) apply(")
	start := strings.Index(body, "f.acceptance.Start(")
	if start < 0 {
		t.Fatal("apply does not call acceptance.Start; this guard is reading the wrong function")
	}
	// 1000 bytes reaches past the branch's own explanatory comment (924 bytes
	// to its "f.rollback(state, user)") while stopping well short of the next
	// occurrence of that call, in the kernel-write branch's own rollback
	// (2950 bytes away) — so widening it cannot make this pass by finding the
	// wrong branch's rollback.
	branch := body[start:min(start+1000, len(body))]
	if !strings.Contains(branch, "f.rollback(state, user)") {
		t.Error("the acceptance.Start error path returns without rolling back. PromoteStaged " +
			"has already run, so Current holds a set nobody confirmed — and the next boot or " +
			"resume installs it with no window, because Current is assumed to have survived one")
	}
}
