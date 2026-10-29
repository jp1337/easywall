package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// The two files the core writes are readable by root and nobody else.
//
// Both were 0640, created by a root daemon in a directory whose group is
// easywall — the user the network-facing web process runs as. Neither file is
// ever opened by that process: audit entries reach it over the socket
// (GET_AUDIT_LOG) and the last-apply time comes with the status. So the group
// read was privilege granted to a process that does not use it, over the record
// of who changed the firewall.
//
// gosec says the same thing (G302, G306) and was right; this test is what keeps
// the answer from drifting back. debian/easywall.logrotate recreates the audit
// log with the same mode, which is the other half of it.
func TestCoreWritesItsFilesForRootOnly(t *testing.T) {
	dir := t.TempDir()

	t.Run("audit log", func(t *testing.T) {
		path := filepath.Join(dir, "audit.log")
		WriteAuditLog(path, "apply_started", "all", "", "tester")
		assertMode(t, path, 0600)
	})

	t.Run("last-apply marker", func(t *testing.T) {
		path := filepath.Join(dir, "last_apply")
		f := &Firewall{cfg: &Config{}}
		f.cfg.DataDir = dir
		f.setLastApply(time.Now())
		assertMode(t, path, 0600)
	})

	t.Run("applied-config snapshot", func(t *testing.T) {
		path := filepath.Join(dir, "applied-config.json")
		if err := writeAppliedConfig(path, shared.AppliedConfig{}); err != nil {
			t.Fatalf("writeAppliedConfig: %v", err)
		}
		assertMode(t, path, 0600)
	})

	t.Run("usage counters", func(t *testing.T) {
		path := filepath.Join(dir, "usage.json")
		if err := NewUsageStore(path).ResetBaselines(); err != nil {
			t.Fatalf("ResetBaselines: %v", err)
		}
		assertMode(t, path, 0600)
	})
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s is %04o, want %04o — the group can read what only root should",
			filepath.Base(path), got, want)
	}
}

// An audit log from before 2.5.0 is 0640 root:easywall, and OpenFile's mode
// only applies at creation: where no logrotate recreates the file — the
// container image ships none — it stayed group-readable for ever, in a log_dir
// the web user can enter. Measured 0640 before and after WriteAuditLog. The
// core tightens it when it starts.
func TestNewDaemon_TightensAnOldAuditLog(t *testing.T) {
	cfg := newTestConfig(t)
	path := cfg.AuditLogPath()
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil { // #nosec G302 -- the pre-2.5.0 mode, under test
		t.Fatal(err)
	}
	WriteAuditLog(path, "apply_started", "all", "", "tester")
	_, _ = NewDaemon(cfg) // a failure past the log directory is not this test's concern
	assertMode(t, path, 0600)
}
