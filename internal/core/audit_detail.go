package core

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/jp1337/easywall/internal/shared"
)

// The audit log's detail column exists to answer "what changed". Until 2.5.0
// every save wrote an empty string into it, so the column that carries the
// answer was a dash on every line and the log recorded only that *something*
// had been saved. These helpers fill it.
//
// The text is deliberately short and mechanical: this goes into a log file that
// is read with grep as often as in the interface, and a long sentence there
// costs more than it explains.

// maxDetailItems bounds how many names a single entry lists. A bulk import can
// touch every option, and an audit line that runs to a kilobyte is not read.
const maxDetailItems = 6

// describeRuleChange summarises a rule-set save: how many entries were added
// and removed, and — for the address lists, where individual values are what an
// operator looks for — which ones.
func describeRuleChange(ruleType string, before, after shared.Rules) string {
	switch ruleType {
	case "blacklist":
		return describeListChange(before.Blacklist, after.Blacklist)
	case "whitelist":
		return describeListChange(before.Whitelist, after.Whitelist)
	case "tcp":
		return describeCountChange(len(before.TCP), len(after.TCP), "port")
	case "udp":
		return describeCountChange(len(before.UDP), len(after.UDP), "port")
	case "forwarding":
		return describeCountChange(len(before.Forwarding), len(after.Forwarding), "forward")
	case "custom":
		return describeCountChange(len(before.Custom), len(after.Custom), "rule")
	default:
		return ""
	}
}

// describeListChange names the entries that came and went. Addresses are the
// unit an operator reasons about, so "added 203.0.113.7" beats "1 added".
func describeListChange(before, after []string) string {
	added := missingFrom(after, before)
	removed := missingFrom(before, after)

	var parts []string
	if len(added) > 0 {
		parts = append(parts, "added "+joinCapped(added))
	}
	if len(removed) > 0 {
		parts = append(parts, "removed "+joinCapped(removed))
	}
	if len(parts) == 0 {
		return "no change"
	}
	return strings.Join(parts, ", ")
}

// describeCountChange is for rule kinds whose entries are structs — listing
// them would not fit, and the count is what the log can usefully carry.
func describeCountChange(before, after int, unit string) string {
	switch {
	case after > before:
		return fmt.Sprintf("%d %s added (%d total)", after-before, plural(unit, after-before), after)
	case after < before:
		return fmt.Sprintf("%d %s removed (%d total)", before-after, plural(unit, before-after), after)
	default:
		return fmt.Sprintf("%d %s, edited in place", after, plural(unit, after))
	}
}

// describeStructChange lists the fields whose values differ, by their toml name
// where they have one. Used for the option and settings structs, where the
// question is which switch someone moved.
func describeStructChange(before, after interface{}) string {
	changed := changedFields(reflect.ValueOf(before), reflect.ValueOf(after), "")
	if len(changed) == 0 {
		return "no change"
	}
	sort.Strings(changed)
	return "changed " + joinCapped(changed)
}

// changedFields walks two values of the same struct type in step and collects
// the names of the leaves that differ. Nested structs are recursed into so that
// NetworkSettings reports "docker.enabled" rather than "Docker".
func changedFields(before, after reflect.Value, prefix string) []string {
	if before.Kind() != reflect.Struct || after.Kind() != reflect.Struct {
		return nil
	}

	var changed []string
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
			changed = append(changed, changedFields(b, a, name)...)
			continue
		}
		if !reflect.DeepEqual(b.Interface(), a.Interface()) {
			changed = append(changed, name)
		}
	}
	return changed
}

// missingFrom returns the items of a that do not appear in b.
func missingFrom(a, b []string) []string {
	seen := make(map[string]struct{}, len(b))
	for _, v := range b {
		seen[v] = struct{}{}
	}
	var out []string
	for _, v := range a {
		if _, ok := seen[v]; !ok {
			out = append(out, v)
		}
	}
	return out
}

// joinCapped joins names, replacing the tail beyond maxDetailItems with a
// count. The number still tells the reader how much they are not seeing.
func joinCapped(items []string) string {
	if len(items) <= maxDetailItems {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s and %d more",
		strings.Join(items[:maxDetailItems], ", "), len(items)-maxDetailItems)
}

func plural(unit string, n int) string {
	if n == 1 {
		return unit
	}
	return unit + "s"
}
