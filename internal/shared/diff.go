package shared

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// The diff the apply screen shows. Six rule sets and thirty-one options, each
// compared the way the kernel reads it — not one generic comparison, because a
// reordered port list changes nothing and a reordered custom rule changes
// everything.
//
// It lives here, and it is computed in the web process. The privileged side
// gains no new parsing of network-facing input for it, and the core keeps sole
// ownership of *whether* something is pending (FirewallStatus.HasPending). This
// answers *what*.

// DeltaKind is what happened to one entry.
type DeltaKind string

const (
	DeltaAdded   DeltaKind = "added"
	DeltaRemoved DeltaKind = "removed"
	DeltaChanged DeltaKind = "changed"
)

// RuleDelta is one line of the rule diff.
type RuleDelta struct {
	Set   string    `json:"set"` // "tcp" "udp" "blacklist" "whitelist" "forwarding" "custom"
	Kind  DeltaKind `json:"kind"`
	Key   string    `json:"key"`   // "8443", "192.0.2.42", "8080->80/tcp", "#3"
	Label string    `json:"label"` // the port description, when it has one
	From  string    `json:"from"`  // "changed" only
	To    string    `json:"to"`    // "changed" only
}

// ConfigDelta is one line of the configuration drift: a toml key and the two
// values, never a sentence.
type ConfigDelta struct {
	Key  string `json:"key"`
	From string `json:"from"`
	To   string `json:"to"`
}

// DiffRules reports what applying the staged set would change about the running
// one. Within each set the arrivals come first, in the order they are stored,
// then what leaves, in the order it is stored — a preview whose order shifts
// between two loads is one nobody reads twice.
func DiffRules(current, staged Rules) []RuleDelta {
	var out []RuleDelta
	out = append(out, diffPorts("tcp", current.TCP, staged.TCP)...)
	out = append(out, diffPorts("udp", current.UDP, staged.UDP)...)
	out = append(out, diffList("blacklist", current.Blacklist, staged.Blacklist)...)
	out = append(out, diffList("whitelist", current.Whitelist, staged.Whitelist)...)
	out = append(out, diffForwarding(current.Forwarding, staged.Forwarding)...)
	out = append(out, diffCustom(current.Custom, staged.Custom)...)
	return out
}

// diffPorts keys by port: the order rules are listed in does not change what is
// accepted, so a moved row is not a change and a description edit is `changed`
// rather than an add and a remove. A port listed twice is diffed on its first
// entry; the duplicate produces no rule of its own either.
func diffPorts(set string, current, staged []PortRule) []RuleDelta {
	cur := indexPorts(current)
	next := indexPorts(staged)

	var out []RuleDelta
	seen := map[string]bool{}
	for _, r := range staged {
		if seen[r.Port] {
			continue
		}
		seen[r.Port] = true
		before, existed := cur[r.Port]
		switch {
		case !existed:
			out = append(out, RuleDelta{Set: set, Kind: DeltaAdded, Key: r.Port, Label: portDetail(r)})
		case portDetail(before) != portDetail(r):
			out = append(out, RuleDelta{Set: set, Kind: DeltaChanged, Key: r.Port,
				Label: portDetail(r), From: portDetail(before), To: portDetail(r)})
		}
	}
	seen = map[string]bool{}
	for _, r := range current {
		if seen[r.Port] {
			continue
		}
		seen[r.Port] = true
		if _, still := next[r.Port]; !still {
			out = append(out, RuleDelta{Set: set, Kind: DeltaRemoved, Key: r.Port, Label: portDetail(r)})
		}
	}
	return out
}

func indexPorts(rules []PortRule) map[string]PortRule {
	m := make(map[string]PortRule, len(rules))
	for _, r := range rules {
		if _, dup := m[r.Port]; !dup {
			m[r.Port] = r
		}
	}
	return m
}

// portDetail is everything about a port rule that is not its port: what it is
// for, and whether it routes through the SSH brute-force chain. It is what the
// diff shows beside the number and what a "changed" delta carries.
func portDetail(r PortRule) string {
	switch {
	case r.SSH && r.Description == "":
		return "(ssh)"
	case r.SSH:
		return r.Description + " (ssh)"
	default:
		return r.Description
	}
}

// diffList compares an address list as a set, skipping the operator's comments
// and blank spacers. Distinct addresses, order irrelevant — countListEntries
// already established that a comment is not a rule.
func diffList(set string, current, staged []string) []RuleDelta {
	cur := listEntrySet(current)
	next := listEntrySet(staged)

	var out []RuleDelta
	for _, entry := range listEntries(staged) {
		if !cur[entry] {
			out = append(out, RuleDelta{Set: set, Kind: DeltaAdded, Key: entry})
		}
	}
	for _, entry := range listEntries(current) {
		if !next[entry] {
			out = append(out, RuleDelta{Set: set, Kind: DeltaRemoved, Key: entry})
		}
	}
	return out
}

// listEntries returns the entries that are rules, trimmed, in order, without
// repeats.
func listEntries(lines []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range lines {
		if IsListComment(l) {
			continue
		}
		e := strings.TrimSpace(l)
		if seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out
}

func listEntrySet(lines []string) map[string]bool {
	set := map[string]bool{}
	for _, e := range listEntries(lines) {
		set[e] = true
	}
	return set
}

// diffForwarding keys by (protocol, source port): there is one redirect per
// incoming port, so changing where it goes is a change to that redirect and not
// a different rule. The key of a changed entry names the incoming side, which is
// its identity; From and To carry the whole rule either side of the edit.
func diffForwarding(current, staged []ForwardingRule) []RuleDelta {
	cur := indexForwards(current)
	next := indexForwards(staged)

	var out []RuleDelta
	for _, r := range staged {
		id := forwardID(r)
		before, existed := cur[id]
		switch {
		case !existed:
			out = append(out, RuleDelta{Set: "forwarding", Kind: DeltaAdded, Key: forwardText(r)})
		case before.DestPort != r.DestPort:
			out = append(out, RuleDelta{Set: "forwarding", Kind: DeltaChanged, Key: id,
				From: forwardText(before), To: forwardText(r)})
		}
	}
	for _, r := range current {
		if _, still := next[forwardID(r)]; !still {
			out = append(out, RuleDelta{Set: "forwarding", Kind: DeltaRemoved, Key: forwardText(r)})
		}
	}
	return out
}

func indexForwards(rules []ForwardingRule) map[string]ForwardingRule {
	m := make(map[string]ForwardingRule, len(rules))
	for _, r := range rules {
		if _, dup := m[forwardID(r)]; !dup {
			m[forwardID(r)] = r
		}
	}
	return m
}

func forwardID(r ForwardingRule) string {
	return strconv.Itoa(r.SourcePort) + "/" + r.Protocol
}

func forwardText(r ForwardingRule) string {
	return fmt.Sprintf("%d->%d/%s", r.SourcePort, r.DestPort, r.Protocol)
}

// diffCustom compares the custom block as a sequence, because raw nftables rules
// are evaluated in the order they are written — a moved line *is* a change.
//
// The sequence is the lines the kernel will see: applyCustomRules trims and
// skips comments and blanks before handing anything to nft, so "#3" here means
// the third rule that will be evaluated, not the third row of the textarea.
func diffCustom(current, staged []string) []RuleDelta {
	cur, next := customLines(current), customLines(staged)

	var out []RuleDelta
	for i := 0; i < len(next); i++ {
		key := "#" + strconv.Itoa(i+1)
		switch {
		case i >= len(cur):
			out = append(out, RuleDelta{Set: "custom", Kind: DeltaAdded, Key: key, Label: next[i]})
		case cur[i] != next[i]:
			out = append(out, RuleDelta{Set: "custom", Kind: DeltaChanged, Key: key,
				From: cur[i], To: next[i]})
		}
	}
	for i := len(next); i < len(cur); i++ {
		out = append(out, RuleDelta{Set: "custom", Kind: DeltaRemoved,
			Key: "#" + strconv.Itoa(i+1), Label: cur[i]})
	}
	return out
}

// customLines trims and skips comments and blanks, exactly what
// applyCustomRules does before handing anything to nft, but does not
// deduplicate: a custom rule set may hold the same line twice on purpose (two
// identical accepts are two rules), so the de-duplication listEntries does for
// address lists must not apply here. diffCustom must not pipe this through
// listEntries.
func customLines(lines []string) []string {
	var out []string
	for _, l := range lines {
		if IsListComment(l) {
			continue
		}
		out = append(out, strings.TrimSpace(l))
	}
	return out
}

// DiffConfig reports the drift between the configuration that went into the
// kernel and the configuration as it is now. Firewall options and network
// settings are walked with no prefix of their own, so the keys read the way they
// are written in easywall.toml: "drop_fragments", "ipv6.mode".
func DiffConfig(applied, live AppliedConfig) []ConfigDelta {
	out := structDeltas(reflect.ValueOf(applied.Firewall), reflect.ValueOf(live.Firewall), "")
	return append(out, structDeltas(reflect.ValueOf(applied.Network), reflect.ValueOf(live.Network), "")...)
}

// skippedConfigKeys are leaves DiffConfig deliberately does not report, each for
// a reason that is not "it was inconvenient". The reflection guard reads this
// map, so a field cannot be dropped from the preview without a line here.
var skippedConfigKeys = map[string]string{
	"ipv6.enabled": "the pre-2.5.0 spelling. Config.Normalise translates it into " +
		"ipv6.mode and never writes it back, so a difference here describes the " +
		"file's history rather than anything the kernel will do differently",
}

// structDeltas walks two values of the same struct type in step and returns one
// delta per leaf that differs, named by its toml tag — or its json tag, for the
// structs that carry only those. Nested structs are recursed into, so
// NetworkSettings reports "docker.enabled" rather than "Docker".
//
// This is changedFields with the values kept rather than discarded.
// DescribeStructChange builds the audit line from the same walk, so the two
// cannot disagree about what changed.
func structDeltas(before, after reflect.Value, prefix string) []ConfigDelta {
	if before.Kind() != reflect.Struct || after.Kind() != reflect.Struct {
		return nil
	}

	var out []ConfigDelta
	t := before.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		name := field.Tag.Get("toml")
		if name == "" {
			name = field.Tag.Get("json")
		}
		if name == "" {
			name = strings.ToLower(field.Name)
		}
		name = strings.Split(name, ",")[0]
		if prefix != "" {
			name = prefix + "." + name
		}

		b, a := before.Field(i), after.Field(i)
		if b.Kind() == reflect.Struct {
			out = append(out, structDeltas(b, a, name)...)
			continue
		}
		if _, skip := skippedConfigKeys[name]; skip {
			continue
		}
		if leafChanged(b, a) {
			out = append(out, ConfigDelta{Key: name, From: formatValue(b), To: formatValue(a)})
		}
	}
	return out
}

// leafChanged is reflect.DeepEqual with one exception: two zero-length slices
// never differ, whether or not either is nil. DeepEqual does distinguish them,
// and the config this compares travels through encoding/json before Task 5
// hands it here — a nil slice marshals to `null` and comes back nil, an empty
// one comes back `[]string{}` — so DockerConfig.CustomNetworks and
// RoutingConfig.Networks, both `[]string` and both `[]` in the shipped
// easywall.toml, would report a permanent phantom drift on every installation
// and leave FirewallStatus.HasPending stuck true.
func leafChanged(b, a reflect.Value) bool {
	if b.Kind() == reflect.Slice && a.Kind() == reflect.Slice && b.Len() == 0 && a.Len() == 0 {
		return false
	}
	return !reflect.DeepEqual(b.Interface(), a.Interface())
}

// formatValue renders one leaf for a human. "on"/"off" rather than
// "true"/"false" because that is what the options page calls a switch, and a
// comma-separated list rather than Go's bracketed slice syntax.
func formatValue(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			return "on"
		}
		return "off"
	case reflect.Slice:
		parts := make([]string, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			parts = append(parts, fmt.Sprint(v.Index(i).Interface()))
		}
		if len(parts) == 0 {
			return "none"
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(v.Interface())
	}
}
