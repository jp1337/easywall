package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// What each port rule has carried, and when it last carried anything.
//
// An open port nobody uses is the most common avoidable exposure on a hobby
// server, and until this release nothing in easywall could point at one. The
// kernel has always known — every rule this daemon writes now carries an
// expr.Counter — and this file is the bookkeeping that turns a counter which
// resets on every apply into a date that does not.
//
// The shape is appliedconfig.go's: one JSON file under DataDir, mode 0600,
// written by atomic rename. Only the core reads it; the web process asks over
// the socket.

// UsageStore owns DataDir/usage.json.
//
// mu serialises the read-modify-write, exactly as RulesStore.mu does and for the
// same reason: Collect is called from the ticker goroutine and from an apply,
// and both are a read of the whole file followed by a write of the whole file.
type UsageStore struct {
	mu   sync.Mutex
	path string
}

// NewUsageStore returns a store over path. Nothing is read or created until the
// first call: a daemon that never collects leaves no file, and a missing file
// means "never collected" rather than an error.
func NewUsageStore(path string) *UsageStore {
	return &UsageStore{path: path}
}

// Read returns the stored record. A missing file is an empty result and no
// error — that is what an installation that has not collected yet looks like,
// and it means *nothing observed*, never *nothing happened*.
func (u *UsageStore) Read() (shared.UsageResult, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.read()
}

func (u *UsageStore) read() (shared.UsageResult, error) {
	data, err := os.ReadFile(u.path) // #nosec G304 -- path is built from the daemon's own config
	if os.IsNotExist(err) {
		return shared.UsageResult{Usage: map[string]shared.RuleUsage{}}, nil
	}
	if err != nil {
		return shared.UsageResult{}, fmt.Errorf("read usage: %w", err)
	}
	var res shared.UsageResult
	if err := json.Unmarshal(data, &res); err != nil {
		return shared.UsageResult{}, fmt.Errorf("parse usage: %w", err)
	}
	if res.Usage == nil {
		res.Usage = map[string]shared.RuleUsage{}
	}
	return res, nil
}

// Collect books what the kernel has counted since the last call.
//
// counters is what RuleCounters read; live is the set of rule ids that still
// exist in the stored rules — Current and Staged together, because a rule
// deleted from Staged is still in the kernel and still counting.
//
// Liveness comes from the rules and never from the kernel. Panic mode deletes
// the whole table, so an id absent from counters proves nothing about whether
// the rule exists; pruning on that would throw every history away the moment
// somebody runs `panic` at the console.
func (u *UsageStore) Collect(counters map[string]RuleCounter, live map[string]bool) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	res, err := u.read()
	if err != nil {
		return err
	}
	now := time.Now().UTC()

	for id, kernel := range counters {
		if !live[id] {
			continue
		}
		entry := res.Usage[id]

		// now < baseline means the counter restarted under us: the table was
		// rebuilt by something easywall did not drive — `nft flush ruleset` typed
		// by hand is the case that remains — so everything the new counter holds
		// is new. The interval between the last collect and that flush is lost,
		// and is documented as a known limit rather than defended against: the
		// reconciler already owns that class of event.
		deltaPackets, deltaBytes := kernel.Packets, kernel.Bytes
		if kernel.Packets >= entry.KernelPackets {
			deltaPackets = kernel.Packets - entry.KernelPackets
		}
		if kernel.Bytes >= entry.KernelBytes {
			deltaBytes = kernel.Bytes - entry.KernelBytes
		}

		if deltaPackets > 0 {
			entry.Packets += deltaPackets
			entry.Bytes += deltaBytes
			entry.LastSeen = now
			if entry.FirstSeen.IsZero() {
				entry.FirstSeen = now
			}
		}
		entry.KernelPackets = kernel.Packets
		entry.KernelBytes = kernel.Bytes
		res.Usage[id] = entry
	}

	// Anything the rules no longer name is forgotten, or the file grows for the
	// life of the installation. A rule that is live but absent from counters —
	// panic mode, or a source list that produced no kernel rule — keeps
	// everything it had.
	for id := range res.Usage {
		if !live[id] {
			delete(res.Usage, id)
		}
	}

	res.CollectedAt = now
	return u.write(res)
}

// ResetBaselines sets every baseline to zero and leaves the totals alone.
//
// apply() calls it once the kernel write has returned: the table it just wrote
// has none of the old counts in it, so measuring the next delta from the old
// baseline would book nothing at all until the new counters climbed past the
// old ones — on a busy port, an hour of traffic reported as silence.
//
// The now-below-baseline branch in Collect catches most of this on its own. This
// exists for the case it cannot: a rebuilt rule whose counter overtakes the old
// baseline before the next tick.
func (u *UsageStore) ResetBaselines() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	res, err := u.read()
	if err != nil {
		return err
	}
	for id, entry := range res.Usage {
		entry.KernelPackets, entry.KernelBytes = 0, 0
		res.Usage[id] = entry
	}
	return u.write(res)
}

// write persists res atomically — temporary file, chmod, rename — the shape
// writeAppliedConfig uses, and for the same reason: a crash halfway through must
// not leave a half-file that parses as a record.
func (u *UsageStore) write(res shared.UsageResult) error {
	if res.Usage == nil {
		res.Usage = map[string]shared.RuleUsage{}
	}
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal usage: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(u.path), "usage-*.json.tmp")
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
	// 0600, like the audit log, the last-apply marker and the applied-config
	// snapshot. Only the core reads it; the web process asks over the socket.
	if err := os.Chmod(tmpPath, 0600); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("set mode: %w", err)
	}
	if err := os.Rename(tmpPath, u.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic rename: %w", err)
	}
	return nil
}
