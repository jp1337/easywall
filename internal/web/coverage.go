package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// The fallback hides gaps by design: a key missing from fr.json renders the
// English string, and the page looks finished. That is the right behaviour for
// the operator in front of it and the wrong one for everybody else — a gap
// nobody can see is indistinguishable from no gap. This is the accounting that
// makes it visible again.
//
// English is the yardstick because it is the language every other one falls
// back to; a key that is not in en.json cannot be rendered by anybody.

// Coverage is one language measured against English.
type Coverage struct {
	Lang     string
	Have     int
	Total    int
	Missing  []string
	Reviewed bool
}

// Percent is how much of English this language covers.
//
// It never returns 100 while anything is missing. "100%" is a promise, and a
// language one key short of 10,000 has not kept it — integer division would
// have rounded that up and said so.
func (c Coverage) Percent() int {
	if c.Total == 0 {
		return 0
	}
	p := c.Have * 100 / c.Total
	if p >= 100 && len(c.Missing) > 0 {
		return 99
	}
	return p
}

// LocaleCoverage measures every catalogue in dir against en.json.
func LocaleCoverage(dir string) ([]Coverage, error) {
	english, err := localeTranslations(dir, "en")
	if err != nil {
		return nil, err
	}
	codes, err := LocaleCodes(dir)
	if err != nil {
		return nil, err
	}
	status, err := LoadLocaleStatus(dir)
	if err != nil {
		return nil, err
	}

	var out []Coverage
	for _, code := range codes {
		tr, err := localeTranslations(dir, code)
		if err != nil {
			return nil, err
		}
		c := Coverage{Lang: code, Total: len(english), Reviewed: status[code].Reviewed}
		for id, en := range english {
			t, present := tr[id]
			switch {
			case !present:
				c.Missing = append(c.Missing, id)
			case code != "en" && t == en:
				// Byte-identical to English is not a translation. It renders
				// exactly as the fallback would, so counting it would let
				// somebody raise the number by pasting — which is the failure
				// the roadmap named: the rule becoming a sham because the
				// English text was copied in to make a test green.
				//
				// A handful of terms are legitimately the same in both
				// languages ("Docker", "IP"), so this understates coverage by
				// a percent or two. That is the right direction to be wrong
				// in: overstating is what misleads.
				c.Missing = append(c.Missing, id)
			default:
				c.Have++
			}
		}
		sort.Strings(c.Missing)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Lang < out[j].Lang })
	return out, nil
}

// localeTranslations reads one catalogue as id -> translation, keeping only the
// entries that actually carry text. An empty translation is not a translation,
// so it does not count towards coverage — otherwise a file of 461 blanks would
// report 100%.
func localeTranslations(dir, code string) (map[string]string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, code+".json")) // #nosec G304 -- a code LocaleCodes listed
	if err != nil {
		return nil, fmt.Errorf("read %s.json: %w", code, err)
	}
	var entries []struct {
		ID          string `json:"id"`
		Translation string `json:"translation"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("parse %s.json: %w", code, err)
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.Translation != "" {
			out[e.ID] = e.Translation
		}
	}
	return out, nil
}
