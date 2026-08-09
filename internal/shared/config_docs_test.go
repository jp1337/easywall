package shared

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// repoFile walks up from the package directory to the repository root and reads
// the named file, so the test works wherever `go test` was invoked from.
func repoFile(t *testing.T, parts ...string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join(parts...)
	for i := 0; i < 5; i++ {
		if data, err := os.ReadFile(filepath.Join(dir, rel)); err == nil {
			return string(data)
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("could not locate %s above the package directory", rel)
	return ""
}

// tomlKeys returns every toml tag on a struct, recursing into nested structs.
func tomlKeys(t reflect.Type) []string {
	var keys []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name := strings.Split(f.Tag.Get("toml"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			keys = append(keys, tomlKeys(ft)...)
			continue
		}
		keys = append(keys, name)
	}
	return keys
}

// A configuration key that exists in the code and nowhere in the documentation
// is a setting an operator can only find by reading the source. This is how the
// module count in the README came to say nine when there were twelve, and how
// three of them shipped documented as working while producing no rule at all.
//
// The list is derived from the structs, so adding a key without documenting it
// fails here rather than shipping.
func TestEveryConfigKeyIsDocumented(t *testing.T) {
	docs := repoFile(t, "docs", "configuration.md")

	core := tomlKeys(reflect.TypeOf(CoreConfig{}))
	web := tomlKeys(reflect.TypeOf(WebConfig{}))

	// enabled appears in three sections ([acceptance], [ipv6], [docker]); the
	// obsolete ipv6.enabled is described under its own heading.
	for _, key := range append(core, web...) {
		if !strings.Contains(docs, "`"+key+"`") {
			t.Errorf("configuration.md does not document %q", key)
		}
	}
}

// The JSON Schemas drive editor validation, and both files set
// additionalProperties: false — so a key the schema does not know is reported as
// invalid in the operator's editor even though the daemon accepts it. That is
// exactly what happened to ipv6.mode and demo_mode.
func TestEveryConfigKeyIsInTheSchema(t *testing.T) {
	for _, tc := range []struct {
		schema string
		typ    reflect.Type
	}{
		{repoFile(t, "docs", "schemas", "easywall.schema.json"), reflect.TypeOf(CoreConfig{})},
		{repoFile(t, "docs", "schemas", "web.schema.json"), reflect.TypeOf(WebConfig{})},
	} {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(tc.schema), &parsed); err != nil {
			t.Fatalf("schema does not parse: %v", err)
		}
		for _, key := range tomlKeys(tc.typ) {
			if !strings.Contains(tc.schema, `"`+key+`"`) {
				t.Errorf("%s is not described by its JSON Schema", key)
			}
		}
	}
}
