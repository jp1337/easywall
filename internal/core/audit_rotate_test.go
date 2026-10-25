package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The audit log is low-volume and relies on logrotate on the deb. The container
// image ships no logrotate, so without an in-process bound the file the
// privileged core appends to for ever would grow unbounded there. This asserts
// the backstop fires and keeps the newest entries.
func TestWriteAuditLog_BackstopBoundsAnUnrotatedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")

	orig := auditLogMaxBytes
	auditLogMaxBytes = 4 << 10 // 4 KiB, so a few hundred entries trip it
	defer func() { auditLogMaxBytes = orig }()

	const n = 2000
	for i := 0; i < n; i++ {
		// A distinctive, increasing detail so we can tell newest from oldest.
		WriteAuditLog(path, "rules_saved", "ssh", "entry-"+pad(i), "web")
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() >= auditLogMaxBytes {
		t.Fatalf("audit log is %d bytes, not bounded under the %d-byte cap", fi.Size(), auditLogMaxBytes)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// The newest entry must survive; the oldest must have been rotated out.
	if !strings.Contains(string(data), "entry-"+pad(n-1)) {
		t.Error("the newest entry was lost in rotation")
	}
	if strings.Contains(string(data), "entry-"+pad(0)) {
		t.Error("the oldest entry was kept; the file was not actually rotated")
	}
	// Every surviving line must still be a whole, parseable entry — the rotation
	// must cut on a line boundary.
	for _, l := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		var e shared.AuditLogEntry
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			t.Fatalf("rotation left a partial line: %q", l)
		}
	}
}

// TestWriteAuditLog_KeepsFileUnderTheCapIntact is the control: below the cap,
// nothing is rotated and every entry is kept.
func TestWriteAuditLog_KeepsFileUnderTheCapIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	for i := 0; i < 50; i++ {
		WriteAuditLog(path, "options_saved", "", "entry-"+pad(i), "web")
	}
	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 50 {
		t.Fatalf("expected 50 entries intact under the cap, got %d", len(lines))
	}
	if !strings.Contains(string(data), "entry-"+pad(0)) {
		t.Error("an entry well under the cap was dropped")
	}
}

func pad(i int) string {
	s := make([]byte, 0, 6)
	d := []byte{byte('0' + (i/100000)%10), byte('0' + (i/10000)%10), byte('0' + (i/1000)%10),
		byte('0' + (i/100)%10), byte('0' + (i/10)%10), byte('0' + i%10)}
	return string(append(s, d...))
}
