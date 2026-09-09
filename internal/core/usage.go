package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
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

	// lastParseErr is the last unparseable-file message reported, so a file that
	// stays corrupt in the same way is one journal line rather than one per tick.
	// Guarded by mu: every caller of read() holds it. The shape is
	// Firewall.warnAppliedConfigErrOnce's, which exists for the same reason.
	lastParseErr string
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
		// Warned and treated as unrecorded, not returned as an error, and this is
		// the difference between a file that heals and one that never does.
		//
		// Collect reads before it writes. A hard error here therefore made the bad
		// file permanent: every tick read it, failed, logged, and returned before
		// reaching the write that would have replaced it — one warning per
		// interval for the life of the installation, and *Last used* rendering an
		// em dash for every rule for ever, with no way out short of deleting the
		// file by hand. A truncated write on a host that lost power is enough to
		// get there.
		//
		// So the record is dropped and the next collect writes a fresh one. What
		// that costs is the stored history: totals, first-seen and last-seen dates
		// start again, which is what the file being unreadable already meant in
		// practice. It is the answer both sibling state files give — readLastApply
		// warns and returns the zero time, appliedConfig warns and reports
		// "not recorded" — and the reason is the same: this is bookkeeping for a
		// nicety, and it must never be the thing that stops working.
		//
		// An I/O error above stays an error deliberately. A file that cannot be
		// read for want of permission is not corrupt, the write will fail too, and
		// overwriting on that basis would throw away a perfectly good record.
		if msg := err.Error(); msg != u.lastParseErr {
			u.lastParseErr = msg
			slog.Warn("the usage record could not be read and is being started again; "+
				"Last used will report nothing until the next collect",
				"path", u.path, "error", err)
		}
		return shared.UsageResult{Usage: map[string]shared.RuleUsage{}}, nil
	}
	u.lastParseErr = ""
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
	//
	// A reserved id is easywall's own accounting and is not a port rule, so it is
	// dropped here whatever liveness says about it. This loop is the only place
	// the prefix is checked, and it is the right one: it both keeps a reserved id
	// out of the file this call writes and clears one a version without the
	// prefix already put there. A guard in the booking loop above would do the
	// first and not the second, and no test could tell it from a comment — the
	// booked entry is deleted here before anything can read it.
	//
	// It is not filtered out of RuleCounters instead, because the health check
	// reads that map and the established rule's counter is the whole signal it
	// looks at.
	for id := range res.Usage {
		if IsReservedRuleID(id) || !live[id] {
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

// CollectUsage reads the kernel counters once and books what has arrived since
// the last call.
//
// Errors are returned rather than logged here, so the two callers can each say
// something true about their own situation: the ticker logs and carries on, and
// apply() logs and applies anyway — a bookkeeping file that cannot be written is
// no reason to refuse to change the firewall.
func (f *Firewall) CollectUsage() error {
	counters, err := f.nft.RuleCounters()
	if err != nil {
		return fmt.Errorf("read the kernel counters: %w", err)
	}
	live, err := f.liveRuleIDs()
	if err != nil {
		return err
	}
	return f.usage.Collect(counters, live)
}

// liveRuleIDs is every rule id the stored rules still name.
//
// Current *and* Staged: a rule deleted from Staged is still in the kernel and
// still counting until the next apply, and forgetting its history the moment
// somebody unchecks it in the interface — before they have applied, and while
// they can still change their mind — would be the wrong answer twice over.
// Backup is deliberately not included: a rule that is only in Backup is not in
// the kernel, and a rollback that brings it back gets a fresh history, which is
// the honest answer for a rule nothing has been counting.
func (f *Firewall) liveRuleIDs() (map[string]bool, error) {
	state, err := f.rules.GetState()
	if err != nil {
		return nil, fmt.Errorf("read rules for the usage collector: %w", err)
	}
	live := map[string]bool{}
	for _, set := range []shared.Rules{state.Current, state.Staged} {
		for _, list := range [][]shared.PortRule{set.TCP, set.UDP} {
			for _, r := range list {
				if r.ID != "" {
					live[r.ID] = true
				}
			}
		}
	}
	return live, nil
}

// Usage is what GET_USAGE answers with: the stored record, read from the file
// and never from the kernel.
//
// A netlink read here would queue behind the nft mutex, which an apply holds for
// up to NftTimeout — thirty seconds — and the web process's deadline for this
// command is five. The ticker and apply() do the collecting; this is a file read.
func (f *Firewall) Usage() (shared.UsageResult, error) {
	return f.usage.Read()
}

// collectUsageBeforeWrite books the counters immediately before an apply
// destroys them.
//
// nft.Apply deletes and rebuilds the table, and every counter in it goes back to
// zero. Whatever arrived since the last tick is in those counters and nowhere
// else, so reading them here is the difference between losing five minutes of
// traffic per apply and losing none. A failure is logged and swallowed: the
// operator asked for an apply, and a bookkeeping file is not a reason to refuse
// one.
func (f *Firewall) collectUsageBeforeWrite() {
	if err := f.CollectUsage(); err != nil {
		slog.Warn("could not read the port counters before applying; the traffic since the "+
			"last collect will not be counted", "error", err)
	}
}

// resetUsageBaselines follows the kernel write. See UsageStore.ResetBaselines.
//
// A tick's CollectUsage can be blocked on the nft mutex nft.Apply is holding —
// for up to NftTimeout — and unblock the instant apply reaches this call. If
// that tick's Collect commits first, it writes a correct fresh baseline for the
// table apply just rebuilt, and this reset then zeroes it anyway, so the next
// collect measures its delta against 0 and double-books the handful of packets
// the tick already booked. Collect and ResetBaselines are each internally
// serialised but give no ordering guarantee between them, and -race cannot see
// this: it is a logic race, not a data race.
//
// Left as-is rather than fixed. The stored packet and byte totals run slightly
// high, once, and only when a tick lands in that exact instant — nothing an
// operator can act on, because GET_USAGE's reply carries LastSeen and nothing
// else (see RuleUsage's doc comment); the tick's own Collect already set
// LastSeen correctly before the double-booking happens. The cure costs more
// than the defect: closing it needs Collect and ResetBaselines serialised
// against each other, or a reset that merges with a baseline written since the
// collect it follows, rather than zeroing unconditionally — either is a new
// lock straddling the apply path, which does not otherwise take one for a
// nicety counter.
func (f *Firewall) resetUsageBaselines() {
	if err := f.usage.ResetBaselines(); err != nil {
		slog.Warn("could not reset the usage baselines after writing the rules; the next "+
			"collect may under-count", "error", err)
	}
}

// collectUsagePeriodically reads the counters every UsageInterval until quit.
//
// It exists so "last used" has a resolution that does not depend on somebody
// opening the page. Without it the only collect is the one apply() runs, and a
// port used between two applies — which is every port, on a machine whose rules
// are not being changed — would be dated at the next apply or not at all.
//
// An interval of zero stops the ticker and nothing else: the apply-time collect
// still runs, because that call exists to keep the flush from destroying a
// number rather than to sample one.
func (f *Firewall) collectUsagePeriodically(quit <-chan struct{}) {
	interval := f.cfg.UsageInterval()
	if interval <= 0 {
		slog.Info("the usage counter ticker is switched off; Last used will only advance " +
			"when rules are applied")
		return
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-quit:
			return
		case <-tick.C:
			if err := f.CollectUsage(); err != nil {
				slog.Warn("could not read the port counters", "error", err)
			}
		}
	}
}
