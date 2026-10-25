package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// RulesStore manages the three-state rules persistence (current/staged/backup).
// All writes use atomic rename to prevent data corruption on crash.
//
// Every write is a read-modify-write of one file holding all six lists, and the
// daemon handles each socket connection on its own goroutine — so two saves
// that arrive together were both built on the same read and the second one
// wrote the first one away. Measured before this lock existed: 187 of 200
// concurrent saves of two different rule types silently discarded one of them.
// Two browser tabs is all it takes, and the interface said "Changes saved" to
// both.
//
// mu serialises the whole read-modify-write. The exported methods take it; the
// helpers below assume it is held.
type RulesStore struct {
	mu   sync.Mutex
	path string
}

// NewRulesStore creates a RulesStore backed by the given JSON file path.
// If the file does not exist it is initialised with empty rules.
// A stat error that is not "no such file" is fatal rather than ignored. It used
// to fall through to "the file exists, carry on", and the case that produces it
// is precisely the one that matters: a data directory the daemon cannot enter.
// The packaged unit created exactly that — /var/lib/easywall belongs to the web
// user, and CapabilityBoundingSet=CAP_NET_ADMIN leaves root without
// CAP_DAC_OVERRIDE — so the daemon started, announced itself, never wrote
// rules.json, and answered every request with a permission error from deep
// inside the call. A firewall manager that cannot reach its own rules should say
// so at startup, once, in words.
func NewRulesStore(path string) (*RulesStore, error) {
	s := &RulesStore{path: path}
	switch _, err := os.Stat(path); {
	case err == nil:
	case os.IsNotExist(err):
		if err := s.save(emptyState()); err != nil {
			return nil, fmt.Errorf("initialise rules store: %w", err)
		}
	default:
		return nil, fmt.Errorf("rules store at %s is unreachable: %w", path, err)
	}

	// Every rule gets an id here, once, and never on a read.
	//
	// This is not a migration step with a version marker and a one-way door: it
	// is a repair that runs at every start and finds nothing to do on all but
	// the first. Backup is included, because a rollback promotes it into
	// Current — a backup of rules with no ids would orphan every counter at the
	// moment a rollback happens, which is the worst moment available.
	//
	// Filling ids on the read path would be wrong and TestRuleIDsAreUniqueAndStable
	// says so: a value generated per read is a different value every read, and
	// every counter would be keyed to a rule that no longer exists by the time
	// it is read back.
	if err := s.backfillIDs(); err != nil {
		return nil, err
	}
	return s, nil
}

// backfillIDs fills any empty rule id in all three sets and writes once, or not
// at all when there was nothing to fill.
func (s *RulesStore) backfillIDs() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.getState()
	if err != nil {
		return fmt.Errorf("read rules for id backfill: %w", err)
	}
	// Each call is added to the accumulator, never short-circuited by it — a
	// `changed || EnsureRuleIDs(...)` here would skip the later sets the moment
	// the first one changed, and Staged/Backup would keep every rule with no id.
	changed := shared.EnsureRuleIDs(&state.Current)
	changed = shared.EnsureRuleIDs(&state.Staged) || changed
	changed = shared.EnsureRuleIDs(&state.Backup) || changed
	if !changed {
		return nil
	}
	if err := s.save(state); err != nil {
		return fmt.Errorf("write back-filled rule ids: %w", err)
	}
	slog.Info("filled in rule ids for the usage counters", "path", s.path)
	return nil
}

// GetState returns the full three-state rules document.
func (s *RulesStore) GetState() (shared.RulesState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getState()
}

func (s *RulesStore) getState() (shared.RulesState, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return shared.RulesState{}, fmt.Errorf("read rules: %w", err)
	}
	var state shared.RulesState
	if err := json.Unmarshal(data, &state); err != nil {
		return shared.RulesState{}, fmt.Errorf("parse rules: %w", err)
	}
	return state, nil
}

// SaveStaged replaces the staged rule set for one rule type.
// ruleType is one of: "tcp", "udp", "blocklist", "allowlist", "forwarding", "custom".
func (s *RulesStore) SaveStaged(ruleType string, rules interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.getState()
	if err != nil {
		return err
	}

	data, err := json.Marshal(rules)
	if err != nil {
		return fmt.Errorf("marshal rules: %w", err)
	}

	switch ruleType {
	case "tcp":
		var v []shared.PortRule
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		state.Staged.TCP = v
	case "udp":
		var v []shared.PortRule
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		state.Staged.UDP = v
	case "blocklist":
		var v []string
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		state.Staged.Blocklist = v
	case "allowlist":
		var v []string
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		state.Staged.Allowlist = v
	case "feeds":
		// Ids only; ValidateRules below refuses one nothing knows and one
		// switched on twice. A feed's contents never pass through here.
		var v []string
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		state.Staged.Feeds = v
	case "forwarding":
		var v []shared.ForwardingRule
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		state.Staged.Forwarding = v
	case "custom":
		var v []string
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		state.Staged.Custom = v
	default:
		return fmt.Errorf("unknown rule type: %s", ruleType)
	}

	// A rule created in the interface arrives with no id; this is where it gets
	// one. Before the validation below, so the duplicate check there is a
	// backstop for a hand-edited file rather than something the interface can
	// trip over.
	shared.EnsureRuleIDs(&state.Staged)

	// Validate before persisting. ImportRules has always done this and
	// SaveStaged never did, so the same malformed address was rejected when it
	// arrived in a file and accepted when it arrived from the form. It was then
	// stored, listed in the interface as blocked, and silently skipped at apply
	// time by the parse guards in nftables.go — an entry that looked enforced
	// and was not.
	//
	// It belongs here rather than only in the web process because the whole
	// design rests on the privileged side not trusting the unprivileged one.
	if err := shared.ValidateRules(state.Staged); err != nil {
		return fmt.Errorf("invalid %s rules: %w", ruleType, err)
	}

	return s.save(state)
}

// HasPendingChanges returns true when applying the staged rules would change
// the firewall — exactly when /apply's preview, shared.DiffRules, lists a row.
//
// It compared the JSON bytes until 2.25.1, so a comment added to the blocklist
// or two allowlist entries swapped put "Unapplied changes" on the dashboard
// while /apply listed nothing. Such an edit is still stored and goes live with
// the next apply; it just changes nothing in the kernel.
func (s *RulesStore) HasPendingChanges() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.getState()
	if err != nil {
		return false, err
	}
	return len(shared.DiffRules(state.Current, state.Staged)) > 0, nil
}

// BackupCurrent copies current → backup before applying new rules.
func (s *RulesStore) BackupCurrent() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.getState()
	if err != nil {
		return err
	}
	state.Backup = state.Current
	return s.save(state)
}

// PromoteStaged copies staged → current (called after nftables apply succeeds).
func (s *RulesStore) PromoteStaged() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.getState()
	if err != nil {
		return err
	}
	state.Current = state.Staged
	return s.save(state)
}

// Rollback restores the last known good rule set as the enforced one, and
// leaves the staged edits alone.
//
// It used to overwrite Staged as well, which threw the operator's work away at
// the worst possible moment: you edit twenty rules, apply, the new set cuts
// your connection, you wait out the window to get back in — and everything you
// wrote is gone, to be redone over the link you have just proved is fragile.
//
// The published apply-flow diagram, on four pages, says "Previous rules are
// back. Nothing staged was lost." Staged is the workbench; a rollback is a
// statement about what is enforced, not about what you are writing. After this
// the interface correctly reports pending changes, because there are some: the
// set you tried to apply, ready to be corrected and applied again.
func (s *RulesStore) Rollback() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.getState()
	if err != nil {
		return err
	}
	state.Current = state.Backup
	return s.save(state)
}

// ExportStaged returns the staged rule set as pretty-printed JSON.
//
// Staged, not current, for two reasons. Import replaces the staged set, so this
// is the half that makes the pair lossless: export, import, and you are back
// where you were. And export-import.md offers this as what to do "before a
// risky change" — advice that is only worth taking if the file contains the
// edits you are about to risk, rather than the rules that were already live.
//
// The two are identical whenever nothing is pending, which is most of the time;
// the difference shows exactly in the case the documentation describes.
func (s *RulesStore) ExportStaged() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.getState()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(state.Staged, "", "  ")
}

// ImportRules validates and replaces the staged rule set from external JSON.
func (s *RulesStore) ImportRules(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var rules shared.Rules
	if err := json.Unmarshal(data, &rules); err != nil {
		return fmt.Errorf("invalid import data: %w", err)
	}

	// Foreign ids are kept — on this host they have no history, which is the
	// true answer. Duplicates inside the imported set are regenerated, first
	// occurrence keeping its id.
	shared.EnsureRuleIDs(&rules)

	if err := shared.ValidateRules(rules); err != nil {
		return fmt.Errorf("import validation failed: %w", err)
	}
	state, err := s.getState()
	if err != nil {
		return err
	}
	state.Staged = rules
	return s.save(state)
}

// save writes state to disk atomically: write tmp file → rename.
func (s *RulesStore) save(state shared.RulesState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, "rules-*.json.tmp")
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
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic rename: %w", err)
	}
	return nil
}

func emptyState() shared.RulesState {
	empty := shared.Rules{
		TCP:        []shared.PortRule{},
		UDP:        []shared.PortRule{},
		Blocklist:  []string{},
		Allowlist:  []string{},
		Forwarding: []shared.ForwardingRule{},
		Custom:     []string{},
	}
	return shared.RulesState{
		Current: empty,
		Staged:  empty,
		Backup:  empty,
	}
}

// WriteAuditLog appends a structured audit log entry to the given log path.
//
// The line is produced by encoding/json, not by fmt's %q. The two agree on
// ordinary text and part company on anything else: %q escapes an invalid UTF-8
// byte as \xNN, which JSON has no such escape for. The reader parses each line
// with encoding/json and skips what it cannot decode, so a line the writer
// produced and the reader rejects would remove an audit entry from the record
// with nothing to show that it had ever been there. One encoder on both sides
// means that cannot happen.
// auditLogMaxBytes bounds the audit log in place. The deb's logrotate is the
// primary bound — daily, 30 days, compressed (debian/easywall.logrotate); this
// is the backstop for an install that has none. The container image ships
// supervisor and nftables but no logrotate, so without this the one low-volume
// file the privileged core appends to for ever grows unbounded on exactly the
// machine the packet log's own rotation exists to protect (packetlog.go: "a
// missing logrotate is a full disk on the one machine that must not fall over").
// Generous on purpose: where logrotate runs it keeps the file far smaller and
// this never fires. A var so a test can lower it.
var auditLogMaxBytes int64 = 16 << 20 // 16 MiB

// auditMu serialises a rotation's read-modify-write against concurrent appends —
// command handling and feed refusals write audit entries from different
// goroutines.
var auditMu sync.Mutex

func WriteAuditLog(logPath, action, ruleType, detail, user string) {
	line, err := json.Marshal(shared.AuditLogEntry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Action:   action,
		RuleType: ruleType,
		Detail:   detail,
		User:     user,
	})
	if err != nil {
		slog.Error("could not encode audit entry", "action", action, "error", err)
		return
	}

	auditMu.Lock()
	defer auditMu.Unlock()
	rotateAuditLogIfLarge(logPath)

	// 0600, and the group bit is gone on purpose. This is the record of who
	// changed the firewall and when, and the only process that reads it is this
	// one: the web interface asks for entries over the socket (GET_AUDIT_LOG)
	// and never opens the file. It was 0640 root:easywall, which handed the
	// whole history to a network-facing process that has no use for it.
	// Reading it by hand is a `sudo` away, and logrotate recreates it the same
	// way — see debian/easywall.logrotate.
	// #nosec G304 -- logPath is cfg.AuditLogPath(), built from log_dir in the
	// daemon's config; no request can name it.
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		slog.Error("could not open audit log", "path", logPath, "error", err)
		return
	}
	defer f.Close() //nolint:errcheck // the write below is what matters; a close error on append adds nothing

	if _, err := f.Write(append(line, '\n')); err != nil {
		slog.Error("could not write audit entry", "path", logPath, "error", err)
	}
}

// rotateAuditLogIfLarge rewrites the audit log down to its newest entries once it
// grows past auditLogMaxBytes, keeping the most recent half so a rotation is rare
// rather than one per following write. It leaves an audit_rotated entry recording
// how many bytes it dropped: an audit log that silently loses its own history is
// worse than a large one, and the eviction can be forced by whoever can generate
// audited actions. Best-effort — any error leaves the file untouched and the
// append proceeds, because losing an entry to a rotation failure is worse than a
// large file. Call under auditMu.
func rotateAuditLogIfLarge(logPath string) {
	fi, err := os.Stat(logPath)
	if err != nil || fi.Size() < auditLogMaxBytes {
		return
	}
	size := fi.Size()
	keep := auditLogMaxBytes / 2

	// Read only the tail, never the whole file. The install this exists for is one
	// that ran a long time without logrotate, so the file can be far past the cap;
	// loading it whole to drop most of it would spike memory on the privileged
	// process the backstop is meant to protect.
	// #nosec G304 -- logPath is cfg.AuditLogPath(), built from log_dir; no request
	// can name it.
	f, err := os.Open(logPath)
	if err != nil {
		return
	}
	buf := make([]byte, keep)
	n, err := f.ReadAt(buf, size-keep)
	_ = f.Close()
	if err != nil && err != io.EOF {
		return
	}
	tail := buf[:n]
	// Start at a line boundary so the oldest kept entry is whole and parseable.
	if i := bytes.IndexByte(tail, '\n'); i >= 0 {
		tail = tail[i+1:]
	}

	// The marker records bytes, not entries: counting entries would mean reading
	// the dropped region, which is exactly the memory the tail read avoids.
	marker, err := json.Marshal(shared.AuditLogEntry{
		Time:   time.Now().UTC().Format(time.RFC3339),
		Action: "audit_rotated",
		Detail: fmt.Sprintf("dropped %d bytes of older history to stay under %d", size-int64(len(tail)), auditLogMaxBytes),
		User:   "core",
	})
	if err != nil {
		return
	}

	tmp, err := os.CreateTemp(filepath.Dir(logPath), ".audit-*.tmp") // 0600 by os.CreateTemp
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // removed only if the rename below did not consume it
	if _, err := tmp.Write(tail); err != nil {
		_ = tmp.Close()
		return
	}
	if _, err := tmp.Write(append(marker, '\n')); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	_ = os.Rename(tmp.Name(), logPath)
}
