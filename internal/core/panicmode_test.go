package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPanicMarker_RoundTrip(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "panic")

	if PanicEngaged(marker) {
		t.Fatal("a marker that was never written must not read as engaged")
	}
	if err := EngagePanic(marker); err != nil {
		t.Fatalf("EngagePanic: %v", err)
	}
	if !PanicEngaged(marker) {
		t.Error("after EngagePanic the marker must read as engaged")
	}
	if err := ClearPanic(marker); err != nil {
		t.Fatalf("ClearPanic: %v", err)
	}
	if PanicEngaged(marker) {
		t.Error("after ClearPanic the marker must be gone")
	}
}

// Engaging twice is what a second `easywall-core panic` does, and it must not
// fail: an operator repeating the command because they were not sure it landed
// is the likeliest way this is ever called twice.
func TestPanicMarker_EngageIsIdempotent(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "panic")
	if err := EngagePanic(marker); err != nil {
		t.Fatalf("first EngagePanic: %v", err)
	}
	if err := EngagePanic(marker); err != nil {
		t.Fatalf("second EngagePanic: %v", err)
	}
	if !PanicEngaged(marker) {
		t.Error("still engaged after two calls")
	}
}

// Clearing a marker that is not there is the state the caller asked for, not an
// error. `easywall-core resume` on a machine that is already filtering must
// report success.
func TestPanicMarker_ClearWithoutMarkerSucceeds(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "panic")
	if err := ClearPanic(marker); err != nil {
		t.Errorf("clearing an absent marker must succeed, got %v", err)
	}
}

// The most dangerous failure this file can have: an unreadable data directory
// reading as "not engaged", which would restart filtering on a machine somebody
// deliberately unfiltered. Anything that is not a clean "no such file" counts
// as engaged.
func TestPanicMarker_UnreadableCountsAsEngaged(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: a 0000 directory is still traversable")
	}
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(dir, 0o000); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	if !PanicEngaged(filepath.Join(dir, "panic")) {
		t.Error("a marker that cannot be read must be treated as engaged")
	}
}

func TestConfig_PanicMarkerPath(t *testing.T) {
	cfg := &Config{}
	cfg.DataDir = "/var/lib/easywall"
	if got, want := cfg.PanicMarkerPath(), "/var/lib/easywall/panic"; got != want {
		t.Errorf("PanicMarkerPath() = %q, want %q", got, want)
	}
}
