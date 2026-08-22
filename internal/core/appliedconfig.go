package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jp1337/easywall/internal/shared"
)

// The configuration that went into the kernel with the rules that are in it.
//
// It exists because half the interface was missing from HasPending. Firewall
// options and network settings are written straight into this daemon's config
// and take effect only at the next apply, so /options could say "Options saved.
// Apply rules to activate changes." while /apply said "The running firewall
// matches what is staged. There is nothing to apply." — and the false one was on
// the page with the button.
//
// Written wherever nft.Apply succeeds and nowhere else: a refused or failed
// apply did not put anything in the kernel, so there is nothing to record.

// readAppliedConfig returns the recorded snapshot. A missing file is "not
// recorded" and not an error — that is what a machine upgraded to 2.10 looks
// like, and it means *unknown*, never *identical*. A file that is there and will
// not parse is a genuine fault and is reported as one.
func readAppliedConfig(path string) (shared.AppliedConfigResult, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is built from the daemon's own config
	if os.IsNotExist(err) {
		return shared.AppliedConfigResult{}, nil
	}
	if err != nil {
		return shared.AppliedConfigResult{}, fmt.Errorf("read applied config: %w", err)
	}
	var cfg shared.AppliedConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return shared.AppliedConfigResult{}, fmt.Errorf("parse applied config: %w", err)
	}
	return shared.AppliedConfigResult{Recorded: true, Config: cfg}, nil
}

// writeAppliedConfig stores the snapshot atomically — write a temporary file,
// then rename — the same shape RulesStore.save uses, and for the same reason: a
// crash halfway through must not leave a half-file that reads as a
// configuration.
//
// 0600, like the audit log and the last-apply marker. Only this process reads
// it; the web process asks for it over the socket.
func writeAppliedConfig(path string, cfg shared.AppliedConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal applied config: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "applied-config-*.json.tmp")
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
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic rename: %w", err)
	}
	return nil
}

// recordAppliedConfig snapshots the configuration that just went into the
// kernel. Call it immediately after an nft.Apply that returned nil.
//
// The error is logged and not returned, deliberately: the rules are live either
// way, and failing an apply that worked because a bookkeeping file could not be
// written would be the tail wagging the dog. What it costs is one apply's worth
// of drift reporting, and the next successful apply corrects it.
func (f *Firewall) recordAppliedConfig() {
	cfg := shared.AppliedConfig{
		Firewall: f.cfg.FirewallOptions(),
		Network:  f.cfg.NetworkSettings(),
	}
	if err := writeAppliedConfig(f.cfg.AppliedConfigPath(), cfg); err != nil {
		slog.Warn("could not record the configuration that went into the kernel",
			"path", f.cfg.AppliedConfigPath(), "error", err)
	}
}

// appliedConfig returns what recordAppliedConfig last stored. A read failure is
// reported as "not recorded" after a warning: an unreadable snapshot must not
// invent a pending change, and it must not hide one either — it makes the answer
// unknown, which is what "not recorded" means everywhere else in this file.
func (f *Firewall) appliedConfig() shared.AppliedConfigResult {
	res, err := readAppliedConfig(f.cfg.AppliedConfigPath())
	if err != nil {
		slog.Warn("cannot read the recorded configuration; treating it as unrecorded",
			"path", f.cfg.AppliedConfigPath(), "error", err)
		return shared.AppliedConfigResult{}
	}
	return res
}
