package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// writeRulesFile drops a rules.json in place, as an installation upgrading from
// 2.14 has it: no id anywhere.
func writeRulesFile(t *testing.T, path string, state shared.RulesState) {
	t.Helper()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func readRulesFile(t *testing.T, path string) shared.RulesState {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state shared.RulesState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

// Backup included, deliberately. A rollback restores Backup into Current, and a
// backup whose rules have no ids would orphan every counter the moment it is
// used — at the worst possible moment, which is when a rollback happens.
func TestRuleIDsAreUniqueAndStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	writeRulesFile(t, path, shared.RulesState{
		Current: shared.Rules{TCP: []shared.PortRule{{Port: "22"}, {Port: "80"}}},
		Staged:  shared.Rules{TCP: []shared.PortRule{{Port: "22"}, {Port: "80"}}},
		Backup:  shared.Rules{UDP: []shared.PortRule{{Port: "53"}}},
	})

	s1, err := NewRulesStore(path)
	if err != nil {
		t.Fatalf("NewRulesStore: %v", err)
	}
	// GetState is the read entry point every caller — the web process included
	// — actually uses. A backfill relocated to the read path fires here, not at
	// construction, so this call is what makes that relocation observable.
	firstRead, err := s1.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	first := readRulesFile(t, path)

	ids := map[string]bool{}
	for name, set := range map[string]shared.Rules{
		"current": first.Current, "staged": first.Staged, "backup": first.Backup,
	} {
		for _, list := range [][]shared.PortRule{set.TCP, set.UDP} {
			for _, r := range list {
				if r.ID == "" {
					t.Errorf("%s rule %q has no id after the backfill", name, r.Port)
				}
				if ids[r.ID] {
					t.Errorf("%s rule %q reuses id %q", name, r.Port, r.ID)
				}
				ids[r.ID] = true
			}
		}
	}

	// What GetState hands back must be what is on disk. A read-path backfill
	// fails this immediately: it invents an id in memory for the caller without
	// ever writing it, so the reader's value and the file's value part ways on
	// the very first call, before a second open is even needed.
	if firstRead.Current.TCP[0].ID != first.Current.TCP[0].ID || firstRead.Current.TCP[1].ID != first.Current.TCP[1].ID {
		t.Errorf("GetState disagrees with the file on current tcp ids: %q/%q vs %q/%q",
			firstRead.Current.TCP[0].ID, firstRead.Current.TCP[1].ID,
			first.Current.TCP[0].ID, first.Current.TCP[1].ID)
	}
	if firstRead.Backup.UDP[0].ID != first.Backup.UDP[0].ID {
		t.Errorf("GetState disagrees with the file on the backup id: %q vs %q",
			firstRead.Backup.UDP[0].ID, first.Backup.UDP[0].ID)
	}

	// Stable: a second open must not hand out new values. This is what fails
	// when the backfill is moved to the read path — every read is a new id, and
	// every counter is keyed to a rule that no longer exists by the time it is
	// read back.
	s2, err := NewRulesStore(path)
	if err != nil {
		t.Fatalf("second NewRulesStore: %v", err)
	}
	secondRead, err := s2.GetState()
	if err != nil {
		t.Fatalf("second GetState: %v", err)
	}
	second := readRulesFile(t, path)
	for i := range first.Current.TCP {
		if second.Current.TCP[i].ID != first.Current.TCP[i].ID {
			t.Errorf("current tcp rule %d changed id between opens: %q then %q",
				i, first.Current.TCP[i].ID, second.Current.TCP[i].ID)
		}
		// The same comparison against what GetState hands back, not just what is
		// on disk: a read-path backfill that never persists would still show a
		// stable (empty) file while handing every caller a fresh, different id
		// on every call — this is the check that catches that, distinctly from
		// a backfill removed outright, which hands back the same empty id every
		// time and would pass this comparison while failing the "has no id"
		// checks above.
		if secondRead.Current.TCP[i].ID != firstRead.Current.TCP[i].ID {
			t.Errorf("current tcp rule %d changed id between GetState reads: %q then %q",
				i, firstRead.Current.TCP[i].ID, secondRead.Current.TCP[i].ID)
		}
	}
	if second.Backup.UDP[0].ID != first.Backup.UDP[0].ID {
		t.Error("the backup rule changed id between opens")
	}
	if secondRead.Backup.UDP[0].ID != firstRead.Backup.UDP[0].ID {
		t.Error("the backup rule changed id between GetState reads")
	}
}

// A rule the operator adds in the interface gets its id at the moment it is
// stored, not at the next daemon restart. Without this the Last used column
// reads em dash for every new rule for as long as the daemon stays up.
func TestSaveStaged_GivesANewRuleAnID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	s, err := NewRulesStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveStaged("tcp", []shared.PortRule{{Port: "443", Description: "HTTPS"}}); err != nil {
		t.Fatalf("SaveStaged: %v", err)
	}
	state, err := s.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Staged.TCP[0].ID == "" {
		t.Fatal("a rule saved through SaveStaged has no id")
	}

	// And editing it does not re-key it.
	was := state.Staged.TCP[0].ID
	edited := state.Staged.TCP
	edited[0].Description = "HTTPS — main web"
	if err := s.SaveStaged("tcp", edited); err != nil {
		t.Fatalf("second SaveStaged: %v", err)
	}
	state, _ = s.GetState()
	if state.Staged.TCP[0].ID != was {
		t.Errorf("editing the description changed the id from %q to %q; the counter history is orphaned",
			was, state.Staged.TCP[0].ID)
	}
}

// Foreign ids in an import are kept: on this host they have no history, which
// is the true answer. Duplicates inside the imported set are regenerated.
func TestImportRules_KeepsForeignIDsAndRegeneratesDuplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	s, err := NewRulesStore(path)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"tcp":[
		{"id":"deadbeefcafe","port":"22","description":"SSH"},
		{"id":"deadbeefcafe","port":"80","description":"HTTP"},
		{"port":"443","description":"HTTPS"}
	]}`)
	if err := s.ImportRules(payload); err != nil {
		t.Fatalf("ImportRules: %v", err)
	}
	state, _ := s.GetState()
	got := state.Staged.TCP
	if got[0].ID != "deadbeefcafe" {
		t.Errorf("the first occurrence lost its imported id: %q", got[0].ID)
	}
	if got[1].ID == "deadbeefcafe" || got[1].ID == "" {
		t.Errorf("the duplicate was not regenerated: %q", got[1].ID)
	}
	if got[2].ID == "" {
		t.Error("the rule that arrived without an id did not get one")
	}
}
