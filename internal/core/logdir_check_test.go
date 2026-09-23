package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// A host whose rules survived but whose audit log did not has lost its log
// directory — a container recreated without /var/log/easywall mounted. The
// check must say so, and must not say so after an ordinary logrotate.
func TestLogDirLooksLost(t *testing.T) {
	configured := shared.Rules{TCP: []shared.PortRule{{Port: "22"}}}
	for _, tc := range []struct {
		name  string
		setup func(dir string)
		rules shared.Rules
		want  bool
	}{
		{"fresh install, nothing applied", func(string) {}, shared.Rules{}, false},
		{"rules and a log with history", func(d string) { write(t, d, "audit.log", "x\n") }, configured, false},
		{"rules, no log at all", func(string) {}, configured, true},
		{"rules, an empty log", func(d string) { write(t, d, "audit.log", "") }, configured, true},
		{"rules, an empty log just rotated", func(d string) {
			write(t, d, "audit.log", "")
			write(t, d, "audit.log.1", "x\n")
		}, configured, false},
		{"rules, a compressed rotation", func(d string) {
			write(t, d, "audit.log.2.gz", "x")
		}, configured, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(dir)
			if got := logDirLooksLost(filepath.Join(dir, "audit.log"), tc.rules); got != tc.want {
				t.Errorf("logDirLooksLost = %v, want %v", got, tc.want)
			}
		})
	}
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
