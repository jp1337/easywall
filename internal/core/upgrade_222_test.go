package core

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The fixtures are the v2.22.0 code's own output — see
// internal/shared/renamed_test.go. This file holds the ones the core reads.
func upgradeFixturePath(name string) string {
	return filepath.Join("..", "shared", "testdata", "upgrade-2.22", name)
}

// copyFixture puts a fixture in a temporary directory, for a test whose code
// under test writes the file back.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(upgradeFixturePath(name))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The 2.22 shipped easywall.toml with the blocklist log switched on and its
// limit at 25: both values must arrive, with one warning, and the next save
// must write the new keys and not the old ones.
func TestA222ConfigReadsTheOldLogKeys(t *testing.T) {
	path := copyFixture(t, "easywall.toml")

	var n int
	prev := slog.Default()
	slog.SetDefault(slog.New(countingHandler{n: &n, substr: "before 2.23"}))
	c, err := LoadConfig(path)
	slog.SetDefault(prev)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	o := c.FirewallOptions()
	if !o.LogBlocklist || o.LogBlocklistLimit != 25 {
		t.Errorf("LogBlocklist = %v, limit %d; the 2.22 file says true and 25", o.LogBlocklist, o.LogBlocklistLimit)
	}
	if n != 1 {
		t.Errorf("%d warnings naming the old keys, want exactly one line", n)
	}

	if err := c.SaveFirewallOptions(o); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "log_blocklist_connections_limit = 25") {
		t.Errorf("the saved file does not carry the new key with the value:\n%s", raw)
	}
	if strings.Contains(string(raw), "log_blacklist") {
		t.Errorf("the saved file still carries an old key:\n%s", raw)
	}
}

// Both spellings in one file: the new key is what the operator wrote last.
func TestTheNewLogKeyWinsOverItsOldSpelling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "easywall.toml")
	body := "[firewall]\nlog_blacklist_connections = true\nlog_blocklist_connections = false\n" +
		"log_blacklist_connections_limit = 25\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var n int
	prev := slog.Default()
	slog.SetDefault(slog.New(countingHandler{n: &n, substr: "before 2.23"}))
	c, err := LoadConfig(path)
	slog.SetDefault(prev)
	if err != nil {
		t.Fatal(err)
	}
	if o := c.FirewallOptions(); o.LogBlocklist || o.LogBlocklistLimit != 25 {
		t.Errorf("LogBlocklist = %v, limit %d; want false (the new key) and 25 (only the old one names it)",
			o.LogBlocklist, o.LogBlocklistLimit)
	}
	if n != 1 {
		t.Errorf("%d warnings, want one", n)
	}
}

// A file with only the new keys says nothing.
func TestTheShippedConfigRaisesNoOldKeyWarning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "easywall.toml")
	if err := os.WriteFile(path, []byte("[firewall]\nlog_blocklist_connections = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var n int
	prev := slog.Default()
	slog.SetDefault(slog.New(countingHandler{n: &n, substr: "before 2.23"}))
	_, err := LoadConfig(path)
	slog.SetDefault(prev)
	if err != nil || n != 0 {
		t.Errorf("err %v, %d old-key warnings for a file that has none", err, n)
	}
}

// packets.log survives the upgrade ([packet_log] persist). A 2.22 line says
// rule "blacklist"; the /blocked filter for the blocklist must find it, and
// the rewrite at start must store the new name.
func TestA222PacketLogReplaysUnderTheNewRuleName(t *testing.T) {
	path := copyFixture(t, "packets.jsonl")
	p := NewPacketLog(10)
	if err := p.Persist(path); err != nil {
		t.Fatal(err)
	}
	res, err := p.Query(shared.PacketLogFilter{Rule: "blocklist"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Discarded != 0 || res.Matched != 1 {
		t.Fatalf("matched %d, discarded %d — the 2.22 blocklist line must replay under rule blocklist",
			res.Matched, res.Discarded)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"rule":"blocklist"`) || strings.Contains(string(raw), `"blacklist"`) {
		t.Errorf("the rewritten spill file does not carry the new name: %s", raw)
	}
}

// applied-config.json carries Go field names; 2.22's says LogBlacklist.
func TestA222AppliedConfigReadsTheOldFieldNames(t *testing.T) {
	res, err := readAppliedConfig(upgradeFixturePath("applied-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if f := res.Config.Firewall; !res.Recorded || !f.LogBlocklist || f.LogBlocklistLimit != 25 {
		t.Errorf("recorded %v, LogBlocklist %v, limit %d; the 2.22 snapshot says true and 25",
			res.Recorded, f.LogBlocklist, f.LogBlocklistLimit)
	}
}

// And a snapshot this version writes is not overridden by the fallback.
func TestAnAppliedConfigInTheNewNamesIsReadAsWritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "applied-config.json")
	want := shared.AppliedConfig{Firewall: shared.FirewallOptions{LogBlocklist: true, LogBlocklistLimit: 7}}
	if err := writeAppliedConfig(path, want); err != nil {
		t.Fatal(err)
	}
	res, err := readAppliedConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if f := res.Config.Firewall; !f.LogBlocklist || f.LogBlocklistLimit != 7 {
		t.Errorf("got %+v", f)
	}
}

// The store end to end: a 2.22 rules.json opens, a save from the interface
// migrates the whole file, and nothing staged is lost on the way.
func TestA222RulesFileIsMigratedByTheFirstSave(t *testing.T) {
	path := copyFixture(t, "rules.json")
	s, err := NewRulesStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveStaged("custom", []string{}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "blacklist") || strings.Contains(string(raw), "whitelist") {
		t.Errorf("rules.json still carries an old key after a save:\n%s", raw)
	}
	state, err := s.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Current.Blocklist) != 3 || len(state.Staged.Blocklist) != 4 || len(state.Backup.Blocklist) != 2 ||
		len(state.Current.Allowlist) != 2 || len(state.Backup.Allowlist) != 1 {
		t.Errorf("entries lost in the migration: %+v", state)
	}
}

func TestA222ExportImportsIntoTheStore(t *testing.T) {
	s, err := NewRulesStore(filepath.Join(t.TempDir(), "rules.json"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(upgradeFixturePath("export.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ImportRules(data); err != nil {
		t.Fatalf("a 2.22 export is refused: %v", err)
	}
	state, _ := s.GetState()
	if len(state.Staged.Blocklist) != 4 || len(state.Staged.Allowlist) != 2 {
		t.Errorf("imported blocklist %q, allowlist %q", state.Staged.Blocklist, state.Staged.Allowlist)
	}
}
