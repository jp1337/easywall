package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// StrictLangs are the languages that must stay at exact parity with each other.
// Everything else may have gaps: the missing key renders English, and the gap is
// reported rather than hidden.
//
// Two and not more on purpose. The roadmap's reasoning: with eight languages an
// exact-parity rule blocks every feature from here on until all eight are
// translated — or the rule becomes a sham, because somebody pastes the English
// text in to make the test green. A French interface displaying English while
// the project claims to support French is worse than no translation.
var StrictLangs = []string{"en", "de"}

// statusFile is not a language.
const statusFile = "status.json"

// LocaleStatus records what is known about a translation beyond its contents.
type LocaleStatus struct {
	// Reviewed is whether a human who speaks the language has read it. A draft
	// says so in the switcher; claiming a language nobody checked is the same
	// class of untruth as a stale screenshot.
	Reviewed bool `json:"reviewed"`
}

// LocaleCodes lists the language codes dir holds a catalogue for, sorted.
func LocaleCodes(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read locales dir %s: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || name == statusFile {
			continue
		}
		out = append(out, strings.TrimSuffix(name, ".json"))
	}
	sort.Strings(out)
	return out, nil
}

// LoadLocaleStatus reads locales/status.json.
//
// A missing file is not an error: it yields an empty map, and an absent entry
// counts as *not reviewed*. That is the understating direction — claiming a
// language nobody checked is the failure worth avoiding — and it is also what
// lets the existing tests keep using bundleWith, which writes a catalogue into
// a temp directory that has no status file in it.
func LoadLocaleStatus(dir string) (map[string]LocaleStatus, error) {
	raw, err := os.ReadFile(filepath.Join(dir, statusFile)) // #nosec G304 -- the locales dir this process was configured with
	if errors.Is(err, os.ErrNotExist) {
		return map[string]LocaleStatus{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", statusFile, err)
	}
	var out map[string]LocaleStatus
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", statusFile, err)
	}
	return out, nil
}
