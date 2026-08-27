package shared

import (
	"reflect"
	"strings"
	"testing"
)

// A seventh rule set that DiffRules never looks at would ship as a preview that
// under-reports: the operator reads "what changes", sees six sections, and the
// seventh applies anyway. The list is derived from the struct, so adding a field
// without diffing it fails here.
//
// The part that matters most is the last assertion: a walk that matches nothing
// must fail rather than pass.
func TestDiffRulesReachesEveryRuleSet(t *testing.T) {
	t.Parallel()
	rt := reflect.TypeOf(Rules{})
	if rt.NumField() == 0 {
		t.Fatal("Rules has no fields; the walk below would pass by matching nothing")
	}

	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		set := strings.Split(field.Tag.Get("json"), ",")[0]
		if set == "" {
			t.Fatalf("Rules.%s has no json tag, so the diff has no name for it", field.Name)
		}

		staged := Rules{}
		reflect.ValueOf(&staged).Elem().Field(i).Set(oneEntry(t, field.Type))

		deltas := DiffRules(Rules{}, staged)
		if len(deltas) == 0 {
			t.Errorf("DiffRules ignores Rules.%s (%q): an entry was added and the "+
				"preview reported nothing", field.Name, set)
			continue
		}
		if deltas[0].Set != set {
			t.Errorf("a change to Rules.%s was reported under set %q, want %q",
				field.Name, deltas[0].Set, set)
		}
	}
}

// oneEntry builds a slice holding a single plausible entry of the given slice
// type, whatever that type is. It has to work for a type nobody has written yet
// — that is the point of the guard.
func oneEntry(t *testing.T, typ reflect.Type) reflect.Value {
	t.Helper()
	if typ.Kind() != reflect.Slice {
		t.Fatalf("Rules holds a %s, which this guard cannot build an entry for; "+
			"teach it, do not delete the case", typ)
	}
	slice := reflect.MakeSlice(typ, 1, 1)
	elem := slice.Index(0)
	switch elem.Kind() {
	case reflect.String:
		elem.SetString("198.51.100.7")
	case reflect.Struct:
		for i := 0; i < elem.NumField(); i++ {
			f := elem.Field(i)
			switch f.Kind() {
			case reflect.String:
				f.SetString("9999")
			case reflect.Int:
				f.SetInt(9999)
			case reflect.Bool:
				f.SetBool(true)
			}
		}
	default:
		t.Fatalf("entries of %s are %s, which this guard cannot build", typ, elem.Kind())
	}
	return slice
}

// Every option and every network setting is reached by DiffConfig, or is named
// in skippedConfigKeys with a reason. A thirty-second option the preview omits
// is the same defect as a seventh rule set: the apply screen says nothing
// changes and something does.
func TestDiffConfigReachesEveryOption(t *testing.T) {
	t.Parallel()
	reached := map[string]bool{}

	for _, leaf := range configLeaves(t, reflect.TypeOf(FirewallOptions{}), nil, "") {
		applied := AppliedConfig{}
		live := AppliedConfig{}
		mutateLeaf(t, reflect.ValueOf(&live.Firewall).Elem(), leaf.index)
		assertReached(t, DiffConfig(applied, live), leaf.key, reached)
	}
	for _, leaf := range configLeaves(t, reflect.TypeOf(NetworkSettings{}), nil, "") {
		applied := AppliedConfig{}
		live := AppliedConfig{}
		mutateLeaf(t, reflect.ValueOf(&live.Network).Elem(), leaf.index)
		assertReached(t, DiffConfig(applied, live), leaf.key, reached)
	}

	if len(reached) == 0 {
		t.Fatal("no leaves were walked at all; this guard was passing by matching nothing")
	}
}

type configLeaf struct {
	key   string
	index []int
}

// configLeaves lists every non-struct field of typ with the key DiffConfig will
// name it by, skipping the ones skippedConfigKeys accounts for.
func configLeaves(t *testing.T, typ reflect.Type, index []int, prefix string) []configLeaf {
	t.Helper()
	var out []configLeaf
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		name := strings.Split(f.Tag.Get("toml"), ",")[0]
		if name == "" {
			name = strings.Split(f.Tag.Get("json"), ",")[0]
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		if prefix != "" {
			name = prefix + "." + name
		}
		path := append(append([]int(nil), index...), i)
		if f.Type.Kind() == reflect.Struct {
			out = append(out, configLeaves(t, f.Type, path, name)...)
			continue
		}
		if reason, skip := skippedConfigKeys[name]; skip {
			t.Logf("%s is deliberately not in the preview: %s", name, reason)
			continue
		}
		out = append(out, configLeaf{key: name, index: path})
	}
	return out
}

// mutateLeaf gives the addressed leaf a different value of its own type. What
// value is irrelevant — the walker compares, it does not validate.
func mutateLeaf(t *testing.T, root reflect.Value, index []int) {
	t.Helper()
	v := root
	for _, i := range index {
		v = v.Field(i)
	}
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(!v.Bool())
	case reflect.Int:
		v.SetInt(v.Int() + 1)
	case reflect.String:
		v.SetString(v.String() + "x")
	case reflect.Slice:
		v.Set(reflect.Append(v, reflect.ValueOf("10.0.0.0/8")))
	default:
		t.Fatalf("this guard cannot mutate a %s; teach it, do not skip the field", v.Kind())
	}
}

func assertReached(t *testing.T, deltas []ConfigDelta, key string, reached map[string]bool) {
	t.Helper()
	for _, d := range deltas {
		if d.Key == key {
			reached[key] = true
			return
		}
	}
	t.Errorf("DiffConfig ignores %q: it was changed and the preview reported %v", key, deltas)
}
