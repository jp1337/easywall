package shared

import (
	"encoding/json"
	"fmt"
)

// Until 2.23 the two lists were the blacklist and the whitelist. Everything
// easywall writes uses the new names; the functions in this file are where the
// old ones are still read, because files and URLs written by 2.22 outlive the
// upgrade: rules.json, an export, a bookmarked /blocked filter, a line in
// packets.log, a kernel rule loaded before the first apply.
// TestTheOldListNamesAreGone counts every remaining occurrence of the old words
// and fails on one it was not told about.

// oldListNames maps a pre-2.23 name to its current one.
var oldListNames = map[string]string{
	"blacklist": "blocklist",
	"whitelist": "allowlist",
}

// CurrentListName returns the 2.23 name for a list or packet-log rule named
// the old way, and s unchanged otherwise.
func CurrentListName(s string) string {
	if n, ok := oldListNames[s]; ok {
		return n
	}
	return s
}

// UnmarshalJSON reads a rule set in either spelling: every copy in rules.json,
// an export from 2.22, an import. MarshalJSON is not overridden, so a write
// uses only the new names and the first save after an upgrade migrates the
// file.
//
// A document that names one list both ways is refused rather than merged or
// resolved: only a hand edit produces one, and either choice would drop
// entries the operator can still see in the file.
func (r *Rules) UnmarshalJSON(data []byte) error {
	type plain Rules // no methods, so decoding it does not recurse into this one
	v := struct {
		plain
		Blocklist *[]string `json:"blocklist"`
		Allowlist *[]string `json:"allowlist"`
		OldBlock  *[]string `json:"blacklist"`
		OldAllow  *[]string `json:"whitelist"`
	}{plain: plain(*r)}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	block, err := eitherSpelling("blocklist", "blacklist", v.Blocklist, v.OldBlock, r.Blocklist)
	if err != nil {
		return err
	}
	allow, err := eitherSpelling("allowlist", "whitelist", v.Allowlist, v.OldAllow, r.Allowlist)
	if err != nil {
		return err
	}
	*r = Rules(v.plain)
	r.Blocklist, r.Allowlist = block, allow
	return nil
}

func eitherSpelling(name, old string, cur, legacy *[]string, keep []string) ([]string, error) {
	switch {
	case cur != nil && legacy != nil:
		return nil, fmt.Errorf("the rules name both %q and %q; they are one list since 2.23 — keep %q", name, old, name)
	case cur != nil:
		return *cur, nil
	case legacy != nil:
		return *legacy, nil
	}
	return keep, nil
}
