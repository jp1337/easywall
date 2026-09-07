package shared

import (
	"reflect"
	"testing"
)

// IsEmpty decides whether a host counts as configured, and a field it forgets
// reads as "nothing here" — which is the direction that stops a firewall
// enforcing rules somebody wrote. Same shape as TestDiffRulesReachesEveryRuleSet
// and for the same reason: the list is derived from the struct, not typed out
// beside it.
//
// Verify by mutation: drop any clause from Rules.IsEmpty and the matching
// subtest goes red.
func TestRulesIsEmptyCountsEveryField(t *testing.T) {
	if (Rules{}).IsEmpty() != true {
		t.Fatal("a zero Rules must be empty; nothing below means anything otherwise")
	}

	rt := reflect.TypeOf(Rules{})
	for i := range rt.NumField() {
		field := rt.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			if field.Type.Kind() != reflect.Slice {
				t.Fatalf("%s is a %s, not a slice — IsEmpty counts lengths and cannot "+
					"see this field at all", field.Name, field.Type.Kind())
			}

			// One element of whatever this field holds, so the check is the
			// struct's own shape rather than a fixture that has to be updated.
			var r Rules
			v := reflect.ValueOf(&r).Elem().Field(i)
			v.Set(reflect.MakeSlice(field.Type, 1, 1))

			if r.IsEmpty() {
				t.Errorf("Rules with one %s entry reports IsEmpty() = true\n"+
					"  a host whose only configuration is this rule set would be treated as "+
					"never configured, and its stored rules would stop being enforced at boot",
					field.Name)
			}
		})
	}
}
