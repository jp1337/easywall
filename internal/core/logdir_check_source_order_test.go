package core

import (
	"regexp"
	"testing"
)

// logDirLooksLost has to be the first thing Daemon.Start reads the log
// directory for. The panic-marker block right after it can itself call
// WriteAuditLog(d.cfg.AuditLogPath(), "boot_enforce_failed", …) when the
// marker is unreadable — and that write happens before the boot restore, so a
// boot with both an unreadable marker and a lost log directory would never
// warn about the lost directory if the order were reversed: the marker's own
// audit entry would make audit.log look merely rotated rather than lost.
//
// No runtime test can pin this: TestLogDirLooksLost only exercises the pure
// function, and every runtime Daemon.Start test that reaches this far starts
// with a readable panic marker, so it never actually races the marker's audit
// write against the log-directory check. Reading the source and asserting on
// call order is this package's established idiom for exactly that gap — see
// TestDaemonStart_SourceRestoresBeforeItListens in
// daemon_source_order_test.go, whose daemonStartBody helper this test reuses.
func TestDaemonStart_SourceChecksLogDirBeforeItWritesTheAuditLog(t *testing.T) {
	body := daemonStartBody(t)

	logDirCheck := regexp.MustCompile(`logDirLooksLost\(`)
	auditWrite := regexp.MustCompile(`WriteAuditLog\(`)

	checkAt := logDirCheck.FindAllStringIndex(body, -1)
	writeAt := auditWrite.FindAllStringIndex(body, -1)

	// A pattern matching nothing must fail, not pass — see funcBody's own
	// comment on the same failure mode in daemon_source_order_test.go.
	if len(checkAt) == 0 {
		t.Fatal("no call to logDirLooksLost in Daemon.Start: the check was moved out of " +
			"Start, renamed, or removed, and this guard no longer pins the ordering it " +
			"exists for")
	}
	if len(checkAt) != 1 {
		t.Fatalf("want exactly one logDirLooksLost call in Daemon.Start, found %d; this "+
			"test compares the first occurrence and cannot tell which one matters",
			len(checkAt))
	}
	if len(writeAt) == 0 {
		// Nothing writes to the audit log before the boot restore succeeds in the
		// current source — but a Start with no WriteAuditLog call at all is not
		// this test's business to require, so this is only a guard against a glob
		// that silently matched zero, not an assertion that a write must exist.
		return
	}

	if checkAt[0][0] > writeAt[0][0] {
		t.Error("Daemon.Start calls WriteAuditLog before it calls logDirLooksLost. " +
			"The panic-marker block writes boot_enforce_failed to the same audit log " +
			"logDirLooksLost inspects, so a host with an unreadable panic marker and a " +
			"lost log directory would never see the lost-directory warning — the marker's " +
			"own write would make audit.log look merely rotated. logDirLooksLost has to run " +
			"first.")
	}
}
