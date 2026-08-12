package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
