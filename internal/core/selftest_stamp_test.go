package core

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// The stamp exists so the proof runs once per version and kernel rather than on
// every start. Both halves matter: the version because the class of defect being
// prevented is a build defect, and the kernel because a kernel upgrade under an
// unchanged easywall is the case that would otherwise be lost.
func TestStampSkipsOnlyWhenVersionAndKernelMatch(t *testing.T) {
	s := NewStampStore(filepath.Join(t.TempDir(), "selftest.json"))
	if err := s.Write(shared.SelftestStamp{
		Version: "2.17.0", Kernel: "6.12.4-1",
		Result: shared.SelftestPassed, At: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	cases := []struct {
		version, kernel string
		wantStale       bool
	}{
		{"2.17.0", "6.12.4-1", false},
		{"2.18.0", "6.12.4-1", true},
		{"2.17.0", "6.13.0-1", true},
		{"2.18.0", "6.13.0-1", true},
	}
	for _, tc := range cases {
		if got := s.Stale(tc.version, tc.kernel); got != tc.wantStale {
			t.Errorf("Stale(%q, %q) = %v, want %v",
				tc.version, tc.kernel, got, tc.wantStale)
		}
	}
}

// A missing stamp is stale, not an error. That is what an installation that has
// never run the proof looks like.
func TestStampIsStaleWhenAbsent(t *testing.T) {
	s := NewStampStore(filepath.Join(t.TempDir(), "selftest.json"))
	if !s.Stale("2.17.0", "6.12.4-1") {
		t.Error("Stale on a missing stamp = false, want true")
	}
	if got := s.Read().Result; got != "" {
		t.Errorf("Read on a missing stamp = %q, want the zero value", got)
	}
}

// usage.json's rule, for usage.json's reason: a truncated write on a host that
// lost power must not make the file permanently unreadable. It heals and the
// proof runs once more than it had to.
//
// Two fixtures, not one. "syntax error" is the genuinely unreadable file —
// what a truncated write after power loss looks like — and json.Unmarshal
// rejects it before touching any field, so it cannot exercise a guard against
// a *partially filled* stamp. "type mismatch" can: Version parses before
// Kernel's wrong type fails the unmarshal, so it is the only fixture that
// would catch read() returning what it parsed so far instead of the zero
// value. Both must still report the stamp as absent and stale — a
// partially-filled stamp whose Version happened to match would otherwise let
// a host skip the proof on the strength of a corrupt file.
func TestStampHealsFromAnUnreadableFile(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{"syntax error", `{not json`},
		{"type mismatch", `{"version":"2.17.0","kernel":123,"result":"passed"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "selftest.json")
			if err := os.WriteFile(path, []byte(tc.data), 0o600); err != nil {
				t.Fatal(err)
			}
			s := NewStampStore(path)
			if got := s.Read(); got != (shared.SelftestStamp{}) {
				t.Errorf("Read on a corrupt stamp = %+v, want the zero value", got)
			}
			if !s.Stale("2.17.0", "6.12.4-1") {
				t.Error("a corrupt stamp is not stale; it must be")
			}
			if err := s.Write(shared.SelftestStamp{
				Version: "2.17.0", Kernel: "6.12.4-1", Result: shared.SelftestPassed,
			}); err != nil {
				t.Fatalf("Write over a corrupt file: %v", err)
			}
			if got := s.Read().Result; got != shared.SelftestPassed {
				t.Errorf("after the rewrite Result = %q, want passed", got)
			}
		})
	}
}

// A stamp that stays broken is one journal line, not one per health poll.
//
// GET_HEALTH reads the stamp on every call, and health.go says the endpoint is
// deliberately uncached because Docker asks every ten seconds. So an
// unreadable selftest.json warned every 10 s for the life of the installation
// and never healed: only cmd/easywall-core writes the file, so nothing rewrites
// it until the next `selftest --if-stale` at boot. This is the case
// UsageStore.lastParseErr was added for, reached from a second consumer.
//
// The count and not the presence: one warning is right, and it is the second
// through hundredth that make the journal useless.
func TestAnUnreadableStampWarnsOnceAndNotPerPoll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selftest.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}

	var n int
	prev := slog.Default()
	slog.SetDefault(slog.New(countingHandler{n: &n, substr: "self-test stamp"}))
	defer slog.SetDefault(prev)

	s := NewStampStore(path)
	for range 10 {
		s.Read()
	}
	if n != 1 {
		t.Errorf("10 reads of the same corrupt stamp logged %d warnings, want 1: "+
			"Health() reads this on every GET_HEALTH, so one line per read is a "+
			"journal filled at Docker's poll interval for ever", n)
	}

	// And the suppression is per message, not permanent: a file that becomes
	// unreadable in a *different* way is a new fact and has to be said.
	if err := os.WriteFile(path, []byte(`{"version":"2.17.0","kernel":123}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s.Read()
	if n != 2 {
		t.Errorf("a stamp that broke in a new way logged %d warnings in total, want 2: "+
			"the suppression must be per message and not a latch", n)
	}
}

// The mode the sibling state files use. Only the core reads it.
func TestStampIsWrittenPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selftest.json")
	if err := NewStampStore(path).Write(shared.SelftestStamp{
		Version: "2.17.0", Kernel: "x", Result: shared.SelftestPassed,
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 600", got)
	}
}

// KernelRelease must return something, and it must not be a path or contain a
// NUL: it goes into a JSON file and into a comparison.
func TestKernelReleaseIsANonEmptyString(t *testing.T) {
	if got := KernelRelease(); got == "" {
		t.Error("KernelRelease() is empty")
	}
}
