package core

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// An unreadable marker used to log on every PanicEngaged call — and GET_STATUS,
// GET_HEALTH and the unknown-bridge check all call it, so the web's status poll
// and Docker's ten-second healthcheck made the journal one repeated line. It is
// logged once per distinct error now, and again after it has cleared.
func TestPanicEngaged_LogsAnUnreadableMarkerOncePerError(t *testing.T) {
	var n int
	prev := slog.Default()
	slog.SetDefault(slog.New(countingHandler{n: &n, substr: "cannot tell whether panic mode is engaged"}))
	defer slog.SetDefault(prev)

	dir := t.TempDir()
	parent := filepath.Join(dir, "data")
	marker := filepath.Join(parent, "panic")
	if err := os.WriteFile(parent, nil, 0o600); err != nil { // ENOTDIR for the marker
		t.Fatal(err)
	}
	for range 5 {
		if !PanicEngaged(marker) {
			t.Fatal("an unreadable marker must read as engaged")
		}
	}
	if n != 1 {
		t.Errorf("5 calls on one unreadable marker logged %d lines, want 1", n)
	}

	// Cleared, then broken again: that is news, and it is logged.
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if PanicEngaged(marker) {
		t.Fatal("no marker must read as not engaged")
	}
	_ = os.Remove(parent)
	_ = os.WriteFile(parent, nil, 0o600)
	PanicEngaged(marker)
	if n != 2 {
		t.Errorf("an error that cleared and came back logged %d lines in all, want 2", n)
	}
}
