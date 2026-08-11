package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/jp1337/easywall/internal/shared"
)

// repoWebToml is the file the Debian package installs and the container image
// copies to /etc/easywall/web.toml. Testing against the real thing rather than a
// stub is the point: it is three kilobytes of comments, and the comments are
// what this is about.
func repoWebToml(t *testing.T) string {
	t.Helper()
	for _, p := range []string{"../../config/web.toml", "../../../config/web.toml"} {
		if data, err := os.ReadFile(p); err == nil {
			return string(data)
		}
	}
	t.Skip("skipping: config/web.toml not found from the test's working directory")
	return ""
}

// newConfigFrom writes content to a temp file and loads it, with the paths
// pointed somewhere writable.
func newConfigFrom(t *testing.T, content string) (*Config, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "web.toml")
	content = strings.ReplaceAll(content, `ssl_dir     = "/etc/easywall/ssl"`, `ssl_dir     = "`+dir+`/ssl"`)
	content = strings.ReplaceAll(content, `data_dir    = "/var/lib/easywall"`, `data_dir    = "`+dir+`"`)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	// The daemon calls both; ensureSessionKey — the save that starts all of
	// this on a container installation — lives in the second.
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return cfg, path
}

// The shipped file keeps its comments through the save that used to erase them.
//
// On a container installation this save is the first *start*: the shipped
// session_key is a placeholder, ensureSessionKey replaces it and writes back,
// and what an operator opened afterwards was fourteen bare lines.
func TestSave_KeepsTheCommentsOfTheShippedConfig(t *testing.T) {
	original := repoWebToml(t)
	cfg, path := newConfigFrom(t, original)

	if err := cfg.SaveCredentials("admin", "argon2id$hash"); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(written)

	// Every comment line that was in the file is still in it.
	for _, line := range strings.Split(original, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") || len(line) < 12 {
			continue
		}
		if !strings.Contains(text, line) {
			t.Errorf("a comment line was lost:\n  %s", line)
			break
		}
	}

	// And the values actually changed.
	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("the rewritten config no longer parses: %v", err)
	}
	if reloaded.Username != "admin" || reloaded.Password != "argon2id$hash" {
		t.Errorf("after save the file holds username=%q password=%q", reloaded.Username, reloaded.Password)
	}
}

// The session key is the value the container path writes on first start, and it
// is written in place, alignment and trailing comment intact.
func TestSave_ReplacesTheSessionKeyInPlace(t *testing.T) {
	cfg, path := newConfigFrom(t, `
bind_addr   = "0.0.0.0:12227"
socket_path = "/run/easywall/core.sock"
ssl_dir     = "/etc/easywall/ssl"
data_dir    = "/var/lib/easywall"

# Signs the session cookie.
session_key = "CHANGE_ME_32_BYTES_HEX_ENCODED_SESSION_SECRET_HERE_XXXXXXXX"  # replace me
username    = ""
password    = ""
`)
	// LoadConfig has already generated a key over the placeholder and saved.
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(written)

	if !strings.Contains(text, "# Signs the session cookie.") {
		t.Error("the comment above session_key was lost")
	}
	if !strings.Contains(text, "# replace me") {
		t.Error("the trailing comment on the session_key line was lost")
	}
	if strings.Contains(text, "CHANGE_ME_32_BYTES") {
		t.Error("the placeholder session key is still in the file")
	}
	if cfg.SessionKey == "" || len(cfg.SessionKey) < minSessionKeyLen {
		t.Errorf("no usable session key was generated: %q", cfg.SessionKey)
	}
}

// A key the file does not have is appended, not silently dropped.
func TestSave_AppendsAKeyTheFileNeverHad(t *testing.T) {
	cfg, path := newConfigFrom(t, `
bind_addr   = "0.0.0.0:12227"
socket_path = "/run/easywall/core.sock"
ssl_dir     = "/etc/easywall/ssl"
data_dir    = "/var/lib/easywall"
session_key = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[tls]
cert = ""
key  = ""
`)
	if err := cfg.SaveTelemetry(true); err != nil {
		t.Fatalf("SaveTelemetry: %v", err)
	}
	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig after save: %v", err)
	}
	if reloaded.Telemetry == nil || !*reloaded.Telemetry {
		t.Errorf("telemetry did not survive the save: %v", reloaded.Telemetry)
	}
	// Appended above the table, not into it — otherwise it becomes tls.telemetry.
	written, _ := os.ReadFile(path)
	text := string(written)
	if strings.Index(text, "telemetry") > strings.Index(text, "[tls]") {
		t.Errorf("telemetry was appended after [tls], where it means something else:\n%s", text)
	}
}

// wantConfig is a configuration whose managed values are all set to something
// recognisable.
func wantConfig(username string) shared.WebConfig {
	yes := true
	return shared.WebConfig{
		SessionKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Username:   username,
		Password:   "argon2id$hash",
		Telemetry:  &yes,
	}
}

// A file the merge cannot be sure of is left to the encoder rather than guessed
// at. These are the shapes that make an in-place edit ambiguous.
func TestMergeConfig_RefusesWhatItCannotBeSureOf(t *testing.T) {
	for _, tc := range []struct{ name, content string }{
		// Said twice at the top level: which one wins is not ours to decide.
		{"a managed key said twice", "session_key = \"a\"\nusername = \"a\"\nusername = \"b\"\n"},
		{"nothing to edit", ""},
		{"nothing but whitespace", "   \n\n"},
		// Only tables: there is no top level to append to.
		{"only tables", "[tls]\ncert = \"\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := mergeConfig([]byte(tc.content), wantConfig("admin")); ok {
				t.Errorf("this was edited in place and should not have been:\n%s", tc.content)
			}
		})
	}
}

// A managed key inside a table is a different key and must not be rewritten.
func TestMergeConfig_LeavesAKeyInsideATableAlone(t *testing.T) {
	in := `session_key = "old"

[somewhere]
username = "not-the-one"
`
	out, ok := mergeConfig([]byte(in), wantConfig("admin"))
	if !ok {
		t.Fatal("expected the top-level key to be editable")
	}
	if !strings.Contains(string(out), `username = "not-the-one"`) {
		t.Errorf("the key inside [somewhere] was rewritten:\n%s", out)
	}
	if !strings.Contains(string(out), `username = "admin"`) {
		t.Errorf("the top-level username was not appended:\n%s", out)
	}
}

// The guard: whatever comes out has to decode back to what went in.
func TestMergeConfig_ResultAlwaysMeansWhatWasAskedFor(t *testing.T) {
	in := repoWebToml(t)
	out, ok := mergeConfig([]byte(in), wantConfig("admin"))
	if !ok {
		t.Fatal("the shipped config could not be edited in place")
	}
	var got shared.WebConfig
	if _, err := toml.Decode(string(out), &got); err != nil {
		t.Fatalf("the merged file does not parse: %v", err)
	}
	if !sameManagedValues(got, wantConfig("admin")) {
		t.Errorf("the merged file does not say what was asked for: %+v", got)
	}
}

// The fallback keeps a pointer to the documentation, since it has just thrown
// the file's own explanations away.
func TestEncode_SaysWhereTheDocumentationWent(t *testing.T) {
	cfg := &Config{}
	cfg.WebConfig = wantConfig("admin")
	data, err := cfg.encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "easywall-project.org/configuration") {
		t.Errorf("the rebuilt config does not say where the documentation is:\n%s", data)
	}
}
