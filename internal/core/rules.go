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
	return s, nil
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
// ruleType is one of: "tcp", "udp", "blacklist", "whitelist", "forwarding", "custom".
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
	case "blacklist":
		var v []string
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		state.Staged.Blacklist = v
	case "whitelist":
		var v []string
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		state.Staged.Whitelist = v
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

// HasPendingChanges returns true when staged rules differ from current rules.
func (s *RulesStore) HasPendingChanges() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.getState()
	if err != nil {
		return false, err
	}
	cur, err := json.Marshal(state.Current)
	if err != nil {
		return false, err
	}
	staged, err := json.Marshal(state.Staged)
	if err != nil {
		return false, err
	}
	return string(cur) != string(staged), nil
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
		Blacklist:  []string{},
		Whitelist:  []string{},
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

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		slog.Error("could not open audit log", "path", logPath, "error", err)
		return
	}
	defer f.Close() //nolint:errcheck // the write below is what matters; a close error on append adds nothing

	if _, err := f.Write(append(line, '\n')); err != nil {
		slog.Error("could not write audit entry", "path", logPath, "error", err)
	}
}
