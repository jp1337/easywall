package web

import (
	"path/filepath"
	"testing"
)

func TestLocaleCodes_FindsEveryLocaleFileAndNotTheStatusFile(t *testing.T) {
	dir := localesDir(t)
	codes, err := LocaleCodes(dir)
	if err != nil {
		t.Fatalf("LocaleCodes: %v", err)
	}
	want := map[string]bool{"en": true, "de": true}
	for _, c := range codes {
		if c == "status" {
			t.Error("status.json was treated as a language")
		}
		delete(want, c)
	}
	if len(want) > 0 {
		t.Errorf("LocaleCodes missed %v", want)
	}
}

// Every language file has a status entry, and every status entry has a file.
// Without this, a language ships with no statement about whether a human ever
// read it — which is the entire point of the state.
func TestEveryLocaleHasAStatusEntry(t *testing.T) {
	dir := localesDir(t)
	codes, err := LocaleCodes(dir)
	if err != nil {
		t.Fatal(err)
	}
	status, err := LoadLocaleStatus(dir)
	if err != nil {
		t.Fatalf("LoadLocaleStatus: %v", err)
	}
	for _, c := range codes {
		if _, ok := status[c]; !ok {
			t.Errorf("locales/%s.json has no entry in locales/status.json", c)
		}
		delete(status, c)
	}
	for c := range status {
		t.Errorf("locales/status.json names %q, which has no locale file", c)
	}
}

func TestStrictLanguagesAreReviewed(t *testing.T) {
	status, err := LoadLocaleStatus(localesDir(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range StrictLangs {
		if !status[l].Reviewed {
			t.Errorf("%s is a strict language and must be marked reviewed", l)
		}
	}
}

var _ = filepath.Join // keep the import if the helpers move
