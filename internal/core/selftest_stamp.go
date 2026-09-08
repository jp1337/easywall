package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/jp1337/easywall/internal/shared"
	"golang.org/x/sys/unix"
)

// What the self-test proved, and against what.
//
// The class of defect layer C prevents is a build defect: identical on every
// start of the same binary. Proving it once per version is therefore exactly
// enough, and this file is what makes "once" mean once. The kernel is in the
// comparison for the case that would otherwise be lost — a kernel upgrade under
// an unchanged easywall.
//
// The shape is usage.json's and appliedconfig.go's: one JSON file under DataDir,
// mode 0600, written by atomic rename, and an unreadable file heals rather than
// becoming permanent. See UsageStore.read for why that answer and not an error.

type StampStore struct {
	mu   sync.Mutex
	path string
}

func NewStampStore(path string) *StampStore { return &StampStore{path: path} }

// Read returns the stored stamp, or the zero value when there is none or it
// cannot be parsed. Never an error: a stamp that cannot be read means the proof
// has not been recorded, which is what a missing one means too.
func (s *StampStore) Read() shared.SelftestStamp {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}

func (s *StampStore) read() shared.SelftestStamp {
	data, err := os.ReadFile(s.path) // #nosec G304 -- path is built from the daemon's own config
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("the self-test stamp could not be read; the proof will run again",
				"path", s.path, "error", err)
		}
		return shared.SelftestStamp{}
	}
	var stamp shared.SelftestStamp
	if err := json.Unmarshal(data, &stamp); err != nil {
		slog.Warn("the self-test stamp is unreadable and is being started again",
			"path", s.path, "error", err)
		return shared.SelftestStamp{}
	}
	return stamp
}

// Stale reports whether the proof has to run for this version and kernel.
//
// A missing or unreadable stamp is stale. So is one recorded as unprovable: a
// host that could not build a namespace last time may be able to now — the
// capability set changed, or the operator ran the subcommand by hand — and the
// cost of finding out is one attempt per start on a host that stays unable.
// That is the one thing this file deliberately does not optimise, because
// caching "cannot" would hide a host becoming able.
func (s *StampStore) Stale(version, kernel string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	stamp := s.read()
	switch stamp.Result {
	case shared.SelftestPassed, shared.SelftestFailed:
		return stamp.Version != version || stamp.Kernel != kernel
	default:
		return true
	}
}

// Write persists the stamp atomically — temporary file, chmod, rename — the
// shape UsageStore.write uses, and for the same reason.
func (s *StampStore) Write(stamp shared.SelftestStamp) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(stamp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal the self-test stamp: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), "selftest-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("set mode: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic rename: %w", err)
	}
	return nil
}

// KernelRelease is uname -r, read through the syscall rather than a subprocess:
// this runs in the privileged process, and the release may not add one.
func KernelRelease() string {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return "unknown"
	}
	return unix.ByteSliceToString(u.Release[:])
}
