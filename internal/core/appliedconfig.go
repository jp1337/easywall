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
// kernel. Call it immediately after an nft.Apply that returned nil, with the
// exact opts and nets that were passed to that call.
//
// Deliberately not re-read from f.cfg: f.cfg.FirewallOptions() and
// f.cfg.NetworkSettings() take the config RWMutex, and a SAVE_OPTIONS landing
// in the gap between nft.Apply returning and a re-read here would record a
// configuration the kernel does not hold — the exact drift this file exists to
// make visible, reintroduced by the one function that writes the record.
// Passing the same values nft.Apply used is also what makes "one snapshot for
// the whole apply" true rather than aspirational.
//
// The error is logged and not returned, deliberately: the rules are live either
// way, and failing an apply that worked because a bookkeeping file could not be
// written would be the tail wagging the dog. What it costs is one apply's worth
// of drift reporting, and the next successful apply corrects it.
func (f *Firewall) recordAppliedConfig(opts shared.FirewallOptions, nets shared.NetworkSettings) {
	cfg := shared.AppliedConfig{
		Firewall: opts,
		Network:  nets,
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
//
// This is called from Status, and /apply polls GET_STATUS every 2s, so a file
// that exists and will not parse would otherwise produce a warning on every
// single poll for as long as the page stays open — about thirty identical
// journal lines a minute. Logged once per distinct error message instead; a
// different error, or the file becoming readable and failing again later,
// warns again.
func (f *Firewall) appliedConfig() shared.AppliedConfigResult {
	res, err := readAppliedConfig(f.cfg.AppliedConfigPath())
	if err != nil {
		f.warnAppliedConfigErrOnce(err)
		return shared.AppliedConfigResult{}
	}
	f.appliedConfigErrMu.Lock()
	f.appliedConfigLastErr = ""
	f.appliedConfigErrMu.Unlock()
	return res
}

// warnAppliedConfigErrOnce logs err unless the last call reported the exact
// same message, so a snapshot that stays corrupt in the same way is one
// journal line, not one per poll.
func (f *Firewall) warnAppliedConfigErrOnce(err error) {
	msg := err.Error()
	f.appliedConfigErrMu.Lock()
	repeat := f.appliedConfigLastErr == msg
	f.appliedConfigLastErr = msg
	f.appliedConfigErrMu.Unlock()
	if repeat {
		return
	}
	slog.Warn("cannot read the recorded configuration; treating it as unrecorded",
		"path", f.cfg.AppliedConfigPath(), "error", err)
}
