package core

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// Panic mode: the machine is deliberately unfiltered, and it stays that way
// across a restart.
//
// It exists because 2.7 takes away an escape route nobody designed and everyone
// relied on. Before this release nftables was empty after a reboot, so rebooting
// was how an operator got back into a machine their own rules had shut them out
// of. Restoring the rules at startup closes that door, and closing it without
// opening another one would make easywall the thing it promises not to be.
//
// The marker is a file rather than a config key on purpose. It is written by a
// console command, read at startup before anything else happens, and has to
// survive a daemon that cannot parse its own configuration.

// PanicEngaged reports whether panic mode is in force.
//
// Anything that is not a clean "no such file" counts as engaged. The asymmetry
// is deliberate: reading an unreachable data directory as "not engaged" would
// start filtering again on a machine somebody deliberately unfiltered, which is
// the one outcome this file exists to prevent. Refusing to filter is visible in
// the interface and in `easywall-core status`; filtering when told not to is
// discovered by being locked out.
func PanicEngaged(markerPath string) bool {
	_, err := os.Stat(markerPath)
	switch {
	case err == nil:
		return true
	case errors.Is(err, fs.ErrNotExist):
		return false
	default:
		slog.Error("cannot tell whether panic mode is engaged, so assuming it is "+
			"and leaving the firewall alone; fix the permissions on the data "+
			"directory and run `easywall-core resume`",
			"marker", markerPath, "error", err)
		return true
	}
}

// EngagePanic writes the marker. Calling it when the marker already exists is
// success, not a conflict.
//
// Written through a temporary file and a rename so that a marker which exists at
// all is a complete one. A half-written marker would still read as engaged, so
// the atomicity buys little today — it buys everything the moment anything is
// ever stored inside it.
func EngagePanic(markerPath string) error {
	dir := filepath.Dir(markerPath)
	tmp, err := os.CreateTemp(dir, "panic-*.tmp")
	if err != nil {
		return fmt.Errorf("create panic marker: %w", err)
	}
	tmpPath := tmp.Name()

	// 0600: only the core reads it. The web process learns about panic mode over
	// the socket, in the status reply, and never opens this path.
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("set panic marker mode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close panic marker: %w", err)
	}
	if err := os.Rename(tmpPath, markerPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("place panic marker: %w", err)
	}
	return nil
}

// ClearPanic removes the marker. A marker that is not there is the state the
// caller asked for.
func ClearPanic(markerPath string) error {
	if err := os.Remove(markerPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove panic marker: %w", err)
	}
	return nil
}
