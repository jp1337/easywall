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
//
// Both fsyncs are load-bearing, and the promise printed at the console is why.
// `easywall-core panic` tells the operator the machine "stays that way across a
// restart"; a rename that is only in the page cache keeps that promise for a
// clean shutdown and breaks it for the one that matters. On ext4 defaults the
// directory entry can sit unwritten for the whole commit interval, and the next
// thing a locked-out operator does is cut the power — after which the machine
// comes up, finds no marker, restores the rules that shut them out, and this
// release has no escape route left to offer. So: the file's own data first
// (Sync before Close), then the directory that names it (a rename is a
// directory operation, and syncing the file does not commit the entry that
// points at it). Anything less means the marker is durable only by luck.
func EngagePanic(markerPath string) error {
	dir := filepath.Dir(markerPath)
	// 0600 without a Chmod: os.CreateTemp already creates the file with mode
	// 0600, so an explicit tmp.Chmod added nothing but two more ways for
	// `panic` — the command an operator runs when nothing else works — to fail.
	// Only the core reads this path; the web process learns about panic mode
	// over the socket, in the status reply, and never opens it.
	tmp, err := os.CreateTemp(dir, "panic-*.tmp")
	if err != nil {
		return fmt.Errorf("create panic marker: %w", err)
	}
	tmpPath := tmp.Name()

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("flush panic marker: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close panic marker: %w", err)
	}
	if err := os.Rename(tmpPath, markerPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("place panic marker: %w", err)
	}
	if err := syncDir(dir); err != nil {
		// Logged, not returned, and the marker is not undone. The rename has
		// already happened, so panic mode is in force for this boot whatever
		// the directory sync did; all that is uncertain is whether it survives
		// a power cut. Returning an error here would make Firewall.Panic give
		// up *before* tearing the table down — refusing an operator the escape
		// they are asking for right now, on a filesystem quirk, to protect a
		// guarantee about a reboot that has not happened yet. Wrong trade in
		// the wrong direction.
		slog.Error("the panic marker is in place but its directory entry could not be "+
			"flushed to disk; panic mode holds for this boot, but a hard power cut "+
			"before the filesystem commits could lose it and the rules would come "+
			"back on the next start",
			"marker", markerPath, "error", err)
	}
	return nil
}

// syncDir commits a directory entry — the half of a rename that fsyncing the
// file itself does not cover.
func syncDir(dir string) error {
	// #nosec G304 -- dir is the data directory from the daemon's own config
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}

// ClearPanic removes the marker. A marker that is not there is the state the
// caller asked for.
func ClearPanic(markerPath string) error {
	if err := os.Remove(markerPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove panic marker: %w", err)
	}
	return nil
}
