package shared

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
)

// ruleIDShape is the format NewRuleID produces and the only one this product
// accepts: exactly twelve lowercase hex characters.
//
// It is checked rather than assumed because the id is not an internal token. It
// arrives from the web process in the ports form's JSON payload, it is written
// verbatim into the kernel as an nftables comment, and it is read back out of
// the kernel to key the usage counters. Each of those is a place an arbitrary
// string does damage:
//
//   - userdata.Append writes the TLV length as byte(len(data)), an unchecked
//     narrowing. At 254 characters the declared length exceeds the kernel's
//     NFT_USERDATA_MAXLEN and the whole netlink batch is refused — so every
//     apply fails until that one rule is deleted, and the firewall cannot be
//     changed at all. At 255 the length byte wraps to 0 and the id read back is
//     the empty string; at 300 it is 45 bytes of something else. Collect then
//     books the traffic under a key no rule has, and the port reports "never"
//     for ever.
//   - `nft list ruleset` shows it to an operator, and to whatever script reads
//     that output.
var ruleIDShape = regexp.MustCompile(`^[0-9a-f]{12}$`)

// ValidRuleID reports whether id is a well-formed rule id. The empty string is
// not one: callers that accept "no id yet" test for that themselves, because
// EnsureRuleIDs fills those in and omitempty means a rule may legitimately
// arrive without one.
func ValidRuleID(id string) bool {
	return ruleIDShape.MatchString(id)
}

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

// EnsureRuleIDs gives every port rule in r an id it does not already have,
// replaces any id that is not well-formed, and regenerates any id that is
// already in use by an earlier rule. It reports whether it changed anything, so
// a caller can skip a write.
//
// A malformed id is treated exactly like an empty one, and not left for
// ValidateRules to refuse. ValidateRules is the trust boundary the web process
// crosses and it does refuse one — but a rules.json edited by hand would
// otherwise reach nft.Apply, be refused there, and leave a machine whose
// firewall cannot be applied at all until somebody finds the offending line.
// Replacing it costs that rule its counter history, which is the smaller loss
// and the same answer this function already gives a duplicate.
//
// TCP and UDP are walked as one set, because the usage map is keyed by id
// alone: a TCP rule and a UDP rule sharing one would share one counter. TCP
// first, so a collision between the two protocols costs the UDP rule its
// history rather than the TCP one — an arbitrary tie-break, and the note exists
// so that nobody reads the order as meaningful.
//
// It runs on write paths only — NewRulesStore's backfill, SaveStaged,
// ImportRules — and never on a read. An id generated per read is a different
// value on every read, and every counter would be keyed to a rule that no
// longer exists by the time it is read back.
func EnsureRuleIDs(r *Rules) bool {
	// No capacity hint: these hold one entry per port rule, which is dozens,
	// and len(r.TCP)+len(r.UDP) is the only size arithmetic in the tree —
	// CodeQL's size-computation-overflow query flags it as a high-severity
	// alert. The sum cannot overflow an int without exabytes of PortRule, so
	// the alert is wrong, but the hint was worth nothing to begin with and
	// arguing with a scanner over ceremony is the worse trade.
	seen := make(map[string]bool)
	changed := false
	for _, list := range []*[]PortRule{&r.TCP, &r.UDP} {
		for i := range *list {
			id := (*list)[i].ID
			if id == "" || !ValidRuleID(id) || seen[id] {
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
