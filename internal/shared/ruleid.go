package shared

import (
	"crypto/rand"
	"encoding/hex"
)

// NewRuleID returns a fresh rule identifier: twelve lowercase hex characters.
//
// Not a UUID. Thirty-six characters in every export, every rules.json and
// every nftables comment buys nothing here — these ids are compared for
// equality inside one host and never merged across hosts. Not a counter
// either: that needs a high-water mark stored beside the rules, and a rules
// file restored from a backup would then hand out ids that are already in use.
//
// Six bytes is 2^48. At a thousand rules the chance of any collision is under
// one in seven million, and EnsureRuleIDs regenerates a duplicate anyway.
func NewRuleID() string {
	var b [6]byte
	// crypto/rand.Read never returns an error as of Go 1.24 — it panics on a
	// failing system source instead — so there is nothing here to handle.
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// EnsureRuleIDs gives every port rule in r an id it does not already have, and
// regenerates any id that is already in use by an earlier rule. It reports
// whether it changed anything, so a caller can skip a write.
//
// TCP and UDP are walked as one set, because the usage map is keyed by id
// alone: a TCP rule and a UDP rule sharing one would share one counter.
//
// It runs on write paths only — NewRulesStore's backfill, SaveStaged,
// ImportRules — and never on a read. An id generated per read is a different
// value on every read, and every counter would be keyed to a rule that no
// longer exists by the time it is read back.
func EnsureRuleIDs(r *Rules) bool {
	seen := make(map[string]bool, len(r.TCP)+len(r.UDP))
	changed := false
	for _, list := range []*[]PortRule{&r.TCP, &r.UDP} {
		for i := range *list {
			id := (*list)[i].ID
			if id == "" || seen[id] {
				id = NewRuleID()
				for seen[id] {
					id = NewRuleID()
				}
				(*list)[i].ID = id
				changed = true
			}
			seen[id] = true
		}
	}
	return changed
}
