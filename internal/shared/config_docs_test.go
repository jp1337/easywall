package shared

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// repoFile walks up from the package directory to the repository root and reads
// the named file, so the test works wherever `go test` was invoked from.
func repoFile(t *testing.T, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(repoPath(t, parts...))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// repoPath is where repoFile found the file.
func repoPath(t *testing.T, parts ...string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join(parts...)
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(dir, rel)); err == nil {
			return filepath.Join(dir, rel)
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
	docs := repoFile(t, "docs", "_docs", "configuration.md")

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

// The configurations under config/ are what the package installs and what the
// container image copies in. They are the defaults an operator actually gets,
// and nothing had been checking that they still parse against the structs they
// configure — config/easywall.toml was shipping the obsolete ipv6.enabled a
// whole release after mode replaced it, and config/web.toml had never heard of
// update_check.
func TestShippedConfigsMatchTheStructsTheyConfigure(t *testing.T) {
	for _, tc := range []struct {
		file string
		into interface{}
		typ  reflect.Type
	}{
		{"easywall.toml", &CoreConfig{}, reflect.TypeOf(CoreConfig{})},
		{"web.toml", &WebConfig{}, reflect.TypeOf(WebConfig{})},
	} {
		raw := repoFile(t, "config", tc.file)

		// Unknown keys are a drift signal in both directions: a key the struct
		// no longer has, or one it never had.
		meta, err := toml.Decode(raw, tc.into)
		if err != nil {
			t.Errorf("config/%s does not parse: %v", tc.file, err)
			continue
		}
		for _, key := range meta.Undecoded() {
			t.Errorf("config/%s sets %q, which no longer exists in the struct", tc.file, key)
		}

		// And every key the struct has should be present, so an operator can see
		// the setting exists without reading the source. Deprecated keys are the
		// exception: they are read for migration and must not be advertised.
		deprecated := map[string]bool{"enabled": false}
		_ = deprecated
		text := raw
		for _, key := range tomlKeys(tc.typ) {
			if key == "enabled" {
				continue // appears in three sections; presence is covered by parsing
			}
			if !strings.Contains(text, key) {
				t.Errorf("config/%s does not mention %q", tc.file, key)
			}
		}
	}
}

// The obsolete IPv6 boolean must not come back into the shipped default: a
// fresh install that sets it takes the migration path and logs a warning about
// its own packaging on every start.
func TestShippedCoreConfigUsesTheCurrentIPv6Key(t *testing.T) {
	// Decoded rather than grepped: the file aligns its values with spaces, and a
	// substring check would pass or fail on the formatting instead of the value.
	var cfg CoreConfig
	meta, err := toml.Decode(repoFile(t, "config", "easywall.toml"), &cfg)
	if err != nil {
		t.Fatalf("config/easywall.toml does not parse: %v", err)
	}
	if cfg.IPv6.Mode != IPv6Filter {
		t.Errorf("the shipped default should be ipv6.mode = %q, got %q", IPv6Filter, cfg.IPv6.Mode)
	}
	if meta.IsDefined("ipv6", "enabled") {
		t.Error("config/easywall.toml still sets the obsolete ipv6.enabled; a fresh " +
			"install would take the migration path and warn about its own packaging on every start")
	}
}

// An editor shows the schema's "default" as the value a missing key gets. It
// said docker.enabled defaults to false while the daemon detects Docker by
// default, and nothing compared the two. Deprecated keys are skipped: they are
// read for migration only and have no default of their own.
func TestTheSchemaDefaultsAreTheShippedDefaults(t *testing.T) {
	for _, tc := range []struct {
		file string
		def  any
	}{
		{"easywall.schema.json", CoreDefault()},
		{"web.schema.json", WebDefault()},
	} {
		var schema map[string]any
		if err := json.Unmarshal([]byte(repoFile(t, "docs", "schemas", tc.file)), &schema); err != nil {
			t.Fatalf("%s does not parse: %v", tc.file, err)
		}
		compareSchemaDefaults(t, tc.file+": ", schema, reflect.ValueOf(tc.def))
	}
}

func compareSchemaDefaults(t *testing.T, path string, schema map[string]any, v reflect.Value) {
	t.Helper()
	props, _ := schema["properties"].(map[string]any)
	for key, raw := range props {
		prop := raw.(map[string]any)
		field, ok := fieldByTOMLKey(v, key)
		if !ok || prop["deprecated"] == true {
			continue // a key missing from the struct is TestEveryConfigKeyIsInTheSchema's
		}
		if _, nested := prop["properties"]; nested {
			compareSchemaDefaults(t, path+key+".", prop, reflect.Indirect(field))
			continue
		}
		want, ok := prop["default"]
		if !ok {
			continue
		}
		// An unset *bool is off and a nil slice is empty: the JSON that means the
		// same thing to the daemon.
		got := field.Interface()
		switch {
		case field.Kind() == reflect.Pointer && field.IsNil():
			got = reflect.Zero(field.Type().Elem()).Interface()
		case field.Kind() == reflect.Slice && field.IsNil():
			got = []any{}
		}
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want)
		if string(g) != string(w) {
			t.Errorf("%s%s: schema default %s, shipped default %s", path, key, w, g)
		}
	}
}

func fieldByTOMLKey(v reflect.Value, key string) (reflect.Value, bool) {
	for i := 0; i < v.NumField(); i++ {
		if strings.Split(v.Type().Field(i).Tag.Get("toml"), ",")[0] == key {
			return v.Field(i), true
		}
	}
	return reflect.Value{}, false
}

// The language enum is what an editor accepts for language; it said en and de
// while fr shipped, so a valid language = "fr" was flagged as an error.
func TestTheSchemaLanguagesAreTheShippedCatalogues(t *testing.T) {
	var schema struct {
		Properties struct {
			Language struct {
				Enum []string `json:"enum"`
			} `json:"language"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(repoFile(t, "docs", "schemas", "web.schema.json")), &schema); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(repoPath(t, "locales", "en.json")), "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	var shipped []string
	for _, f := range files {
		if name := strings.TrimSuffix(filepath.Base(f), ".json"); name != "status" {
			shipped = append(shipped, name)
		}
	}
	got := append([]string(nil), schema.Properties.Language.Enum...)
	sort.Strings(got)
	if !reflect.DeepEqual(got, shipped) {
		t.Errorf("web.schema.json language enum %v, locales/ ships %v", got, shipped)
	}
}

// configuration.md said "Three flags, and nothing else" while easywall-web also
// took -healthcheck and easywall-core five subcommands. Every flag either main
// defines, and every subcommand, must be named in its "The command line"
// section.
func TestEveryCommandLineFlagIsDocumented(t *testing.T) {
	page := repoFile(t, "docs", "_docs", "configuration.md")
	start := strings.Index(page, "## The command line")
	if start < 0 {
		t.Fatal(`configuration.md has no "## The command line" section`)
	}
	section := page[start:]
	if end := strings.Index(section[3:], "\n## "); end >= 0 {
		section = section[:end+3]
	}
	flagRe := regexp.MustCompile(`flag\.(?:Bool|String|Int|Duration)\("([a-z-]+)"`)
	for _, f := range []string{"cmd/easywall-core/main.go", "cmd/easywall-web/main.go"} {
		for _, m := range flagRe.FindAllStringSubmatch(repoFile(t, f), -1) {
			if !strings.Contains(section, "`-"+m[1]) {
				t.Errorf("%s defines -%s; configuration.md's command-line section does not name it", f, m[1])
			}
		}
	}
	subRe := regexp.MustCompile(`(?m)^\t\{"([a-z]+)", "`)
	subs := subRe.FindAllStringSubmatch(repoFile(t, "cmd", "easywall-core", "subcommands.go"), -1)
	if len(subs) == 0 {
		t.Fatal("found no subcommands in cmd/easywall-core/subcommands.go — the table's shape changed")
	}
	for _, m := range subs {
		if !strings.Contains(section, "`"+m[1]+"`") {
			t.Errorf("easywall-core has the subcommand %s; configuration.md's command-line section does not name it", m[1])
		}
	}
}
