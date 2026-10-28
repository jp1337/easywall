package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jp1337/easywall/internal/shared"
)

// The unconfirmed marker closes the hole a dead core used to leave in the
// acceptance window.
//
// apply promotes Staged into Current on disk before the window opens, and only
// a window that runs to its end — confirmed, timed out, cancelled, or cancelled
// by a graceful stop — puts the file right. A core that died inside the window
// instead (kill -9, OOM, the systemd watchdog, a power cut) left Current
// holding the set nobody had confirmed, and the next start restored it with no
// window at all, because RestoreCurrent's justification was that Current had
// already survived one. Measured in a network namespace: apply 443-only over a
// confirmed 22, kill the process inside the window, start again — the kernel
// held `tcp dport 443` and nothing for 22, the status read idle, and nothing
// would ever roll it back.
//
// So apply writes this marker before the promote and removes it once the
// window has an outcome on disk; RestoreCurrent finding it means the previous
// run never got that far, and reverts Current to Backup — the last set a window
// did confirm — before restoring anything.
//
// The marker also carries the configuration that was in force before the
// apply: the applied-config snapshot as it stood, as JSON. Backup is the rules
// half of "what to go back to" and this is the options-and-network half; the
// live config already holds the operator's new values, so without it a restart
// inside the window rebuilt the confirmed rules with the options under trial.
// An empty marker — written by a core before this, or on a host with no
// snapshot — falls back to the live config, which is what every restart did.

// markUnconfirmed places the marker durably: data, rename, then the directory
// entry, the idiom EngagePanic uses and for its reason — the case this exists
// for is the one where the machine goes down hard.
//
// before is the snapshot of what was live; it is written into the marker when
// it was recorded, and the marker is left empty otherwise. 0600, CreateTemp's
// own mode, like applied-config.json: it holds the same configuration.
func markUnconfirmed(path string, before shared.AppliedConfigResult) error {
	var data []byte
	if before.Recorded {
		var err error
		if data, err = json.Marshal(before.Config); err != nil {
			return fmt.Errorf("encode the configuration in force: %w", err)
		}
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "unconfirmed-*.tmp")
	if err != nil {
		return fmt.Errorf("create unconfirmed marker: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write unconfirmed marker: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("flush unconfirmed marker: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close unconfirmed marker: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("place unconfirmed marker: %w", err)
	}
	// Logged, not returned, as in EngagePanic: the marker is in place for this
	// boot; only its survival of a power cut is uncertain.
	if err := syncDir(dir); err != nil {
		slog.Error("the unconfirmed marker is in place but its directory entry could not be flushed",
			"marker", path, "error", err)
	}
	return nil
}

// clearUnconfirmed removes the marker. A marker left behind is the safe
// failure — the next restore reverts to the last confirmed set — so an error
// here is logged and not returned.
func clearUnconfirmed(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Error("could not remove the unconfirmed marker; the next restart will put back "+
			"the rules in force before the last apply", "marker", path, "error", err)
		return
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		slog.Warn("could not flush the removal of the unconfirmed marker", "marker", path, "error", err)
	}
}

// revertUnconfirmed undoes an apply whose window a previous run of the core
// never closed, and reports whether there was one, with the configuration the
// marker says was in force before it. Called by RestoreCurrent with the apply
// slot held, so no apply in this process can own the marker.
func (f *Firewall) revertUnconfirmed() (shared.AppliedConfigResult, bool, error) {
	path := f.cfg.UnconfirmedMarkerPath()
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return shared.AppliedConfigResult{}, false, nil
	} else if err != nil {
		return shared.AppliedConfigResult{}, false, fmt.Errorf("read unconfirmed marker: %w", err)
	}
	// A marker that will not read or parse still reverts the rules — that is
	// the half that may have locked somebody out — and leaves the options to
	// configToRestore's fallback.
	var before shared.AppliedConfigResult
	if data, err := os.ReadFile(path); err != nil { // #nosec G304 -- path is built from the daemon's own config
		slog.Error("could not read the configuration the unconfirmed marker records", "marker", path, "error", err)
	} else if len(data) > 0 {
		if cfg, err := parseAppliedConfig(data); err != nil {
			slog.Error("could not parse the configuration the unconfirmed marker records", "marker", path, "error", err)
		} else {
			before = shared.AppliedConfigResult{Recorded: true, Config: cfg}
		}
	}
	if err := f.rules.Rollback(); err != nil {
		return before, false, fmt.Errorf("revert an apply that was never confirmed: %w", err)
	}
	clearUnconfirmed(path)
	f.acceptance.RecordRolledBack(ReasonCoreStopped)
	WriteAuditLog(f.cfg.AuditLogPath(), "apply_rolledback", "all", ReasonCoreStopped, "core")
	slog.Warn("the core stopped inside an acceptance window; the rules in force before that apply are restored")
	return before, true, nil
}
