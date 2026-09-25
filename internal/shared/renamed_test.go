package shared

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The fixtures in testdata/upgrade-2.22 were written by the v2.22.0 code itself
// (RulesStore.SaveStaged/BackupCurrent/PromoteStaged, ExportStaged, the packet
// log's spill file, writeAppliedConfig), not typed by hand: a fixture that
// agrees with the reader because the same person wrote both proves nothing.
func upgradeFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "upgrade-2.22", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// All three copies — Current, Staged, Backup — hold different lists in the
// fixture, so a reader that migrated one copy and not the others fails here.
func TestA222RulesFileReadsIntoTheNewNames(t *testing.T) {
	var state RulesState
	if err := json.Unmarshal(upgradeFixture(t, "rules.json"), &state); err != nil {
		t.Fatalf("a rules.json written by 2.22 no longer parses: %v", err)
	}
	want := map[string][2][]string{
		"current": {{"# scanners", "203.0.113.9", "198.51.100.0/24"}, {"192.0.2.10", "2001:db8:1::/48"}},
		"staged":  {{"# scanners", "203.0.113.9", "198.51.100.0/24", "2001:db8::/32"}, {"192.0.2.10", "2001:db8:1::/48"}},
		"backup":  {{"# scanners", "203.0.113.9"}, {"192.0.2.10"}},
	}
	for name, got := range map[string]Rules{"current": state.Current, "staged": state.Staged, "backup": state.Backup} {
		if !reflect.DeepEqual(got.Blocklist, want[name][0]) {
			t.Errorf("%s blocklist = %q, want %q", name, got.Blocklist, want[name][0])
		}
		if !reflect.DeepEqual(got.Allowlist, want[name][1]) {
			t.Errorf("%s allowlist = %q, want %q", name, got.Allowlist, want[name][1])
		}
		if len(got.TCP) != 1 || got.TCP[0].Port != "22" {
			t.Errorf("%s tcp = %+v — the rest of the rule set must still decode", name, got.TCP)
		}
	}

	// And it is written back in the new spelling only.
	out, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, old := range []string{`"blacklist"`, `"whitelist"`} {
		if strings.Contains(string(out), old) {
			t.Errorf("the migrated state is written with %s: %s", old, out)
		}
	}
}

// An export is Rules, marshalled — the same reader, a different file.
func TestA222ExportImports(t *testing.T) {
	var r Rules
	if err := json.Unmarshal(upgradeFixture(t, "export.json"), &r); err != nil {
		t.Fatalf("an export from 2.22 no longer parses: %v", err)
	}
	if want := []string{"# scanners", "203.0.113.9", "198.51.100.0/24", "2001:db8::/32"}; !reflect.DeepEqual(r.Blocklist, want) {
		t.Errorf("blocklist = %q, want %q", r.Blocklist, want)
	}
	if want := []string{"192.0.2.10", "2001:db8:1::/48"}; !reflect.DeepEqual(r.Allowlist, want) {
		t.Errorf("allowlist = %q, want %q", r.Allowlist, want)
	}
	if err := ValidateRules(r); err != nil {
		t.Errorf("the imported 2.22 export does not validate: %v", err)
	}
}

func TestARulesDocumentNamingAListBothWaysIsRefused(t *testing.T) {
	for _, doc := range []string{
		`{"blocklist":["203.0.113.9"],"blacklist":["198.51.100.1"]}`,
		`{"allowlist":[],"whitelist":["192.0.2.10"]}`,
	} {
		var r Rules
		err := json.Unmarshal([]byte(doc), &r)
		if err == nil {
			t.Errorf("%s was accepted as %+v; one list under two names must be refused", doc, r)
			continue
		}
		if !strings.Contains(err.Error(), "since 2.23") {
			t.Errorf("%s: the refusal does not say why: %v", doc, err)
		}
	}
}

// The new names, the ordinary case, and a document that omits a list: the
// custom decoder must not turn an absent key into an empty list, or a partial
// document decoded into a populated value would erase it.
func TestRulesDecodeTheNewNamesAndKeepWhatIsAbsent(t *testing.T) {
	r := Rules{Allowlist: []string{"192.0.2.1"}, Custom: []string{"x"}}
	if err := json.Unmarshal([]byte(`{"blocklist":["203.0.113.9"],"tcp":[]}`), &r); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Blocklist, []string{"203.0.113.9"}) || !reflect.DeepEqual(r.Allowlist, []string{"192.0.2.1"}) ||
		!reflect.DeepEqual(r.Custom, []string{"x"}) {
		t.Errorf("got %+v", r)
	}
}

func TestCurrentListName(t *testing.T) {
	for in, want := range map[string]string{
		"blacklist": "blocklist", "whitelist": "allowlist",
		"blocklist": "blocklist", "drop": "drop", "": "",
	} {
		if got := CurrentListName(in); got != want {
			t.Errorf("CurrentListName(%q) = %q, want %q", in, got, want)
		}
	}
}

// A rule 2.22 loaded keeps its prefix until the first apply after the upgrade.
func TestA222LogPrefixDecodesAsTheBlocklist(t *testing.T) {
	if got := RuleFromPrefix("easywall blacklist: "); got != "blocklist" {
		t.Errorf("RuleFromPrefix(2.22's prefix) = %q, want blocklist", got)
	}
}
