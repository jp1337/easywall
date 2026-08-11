package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// readWebSchema loads the JSON Schema taplo.toml points editors at.
func readWebSchema(t *testing.T) map[string]any {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "docs", "schemas", "web.schema.json"))
	if err != nil {
		t.Fatalf("read web.schema.json: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("web.schema.json does not parse: %v", err)
	}
	return schema
}

func schemaRequired(t *testing.T, schema map[string]any) []string {
	t.Helper()
	raw, _ := schema["required"].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("required contains %T, want string", v)
		}
		out = append(out, s)
	}
	return out
}

// The schema must not demand what the daemon supplies for itself.
//
// It listed session_key as required with minLength 64. Neither is true.
// ensureSessionKey generates a key when there is none — installation/manual.md
// tells operators to leave it out for exactly that reason — and it accepts any
// key of minSessionKeyLen or more. Validated with a JSON Schema validator before
// the change: config/web.toml, the file the Debian package installs as the
// template and the container image copies to /etc/easywall/web.toml, was reported
// invalid ("'CHANGE_ME_…' is too short") by the schema published for it.
//
// A schema that is stricter than the program does not protect anyone; it puts a
// red underline under a working configuration, and the next operator's fix is to
// stop pointing their editor at it.
func TestWebSchemaDoesNotRequireWhatTheDaemonGenerates(t *testing.T) {
	schema := readWebSchema(t)

	if req := schemaRequired(t, schema); slices.Contains(req, "session_key") {
		t.Errorf("web.schema.json requires session_key, but ensureSessionKey generates one "+
			"when it is absent and installation/manual.md tells operators to omit it "+
			"(required = %v)", req)
	}

	props, _ := schema["properties"].(map[string]any)
	key, _ := props["session_key"].(map[string]any)
	if key == nil {
		t.Fatal("web.schema.json has no session_key property")
	}
	got, ok := key["minLength"].(float64)
	if !ok {
		t.Fatal("session_key has no numeric minLength")
	}
	if int(got) != minSessionKeyLen {
		t.Errorf("web.schema.json says session_key needs %d characters; the daemon accepts %d "+
			"(minSessionKeyLen). A key between the two is stored, used, and flagged as invalid "+
			"in the operator's editor", int(got), minSessionKeyLen)
	}
}

// socket_path is required unless demo_mode is on, and the schema has to say so
// conditionally rather than absolutely.
//
// Config.Validate skips the socket path when DemoMode is set, because a demo runs
// against an in-memory mock and there is no core to reach. The schema required it
// unconditionally, which made the complete web.toml in installation/demo.md —
// the one the documentation tells a reader to write — invalid against the schema
// published alongside it.
func TestWebSchemaRequiresSocketPathOnlyOutsideDemoMode(t *testing.T) {
	schema := readWebSchema(t)

	if req := schemaRequired(t, schema); slices.Contains(req, "socket_path") {
		t.Errorf("web.schema.json requires socket_path unconditionally; a demo config has no "+
			"core socket and Config.Validate does not ask for one (required = %v)", req)
	}

	// The conditional has to be there, and it has to key on demo_mode.
	cond, _ := schema["if"].(map[string]any)
	if cond == nil {
		t.Fatal("web.schema.json has no if/then/else: socket_path is then either always " +
			"required or never, and both are wrong")
	}
	condProps, _ := cond["properties"].(map[string]any)
	if _, ok := condProps["demo_mode"]; !ok {
		t.Error("the schema's condition does not look at demo_mode, which is the only thing " +
			"that makes socket_path optional")
	}
	els, _ := schema["else"].(map[string]any)
	if !slices.Contains(schemaRequired(t, els), "socket_path") {
		t.Error("the non-demo branch does not require socket_path; outside demo mode " +
			"Config.Validate refuses to start without it")
	}
}

// And the daemon really does accept the config the documentation prints, which is
// the claim the two tests above are protecting.
func TestDemoConfigWithoutSocketPathIsAccepted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "web.toml")
	// The shape of docs/installation/demo.md, socket_path and all pointer fields
	// absent, session_key still the placeholder a reader is told to replace.
	body := `bind_addr   = "0.0.0.0:12227"
ssl_dir     = "` + dir + `/ssl"
data_dir    = "` + dir + `"
session_key = "CHANGE_ME_32_BYTES_HEX_ENCODED_SESSION_SECRET_HERE_XXXXXXXX"
demo_mode   = true
username    = "demo"
password    = ""
`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the documented demo config was refused: %v", err)
	}
	if cfg.SocketPath != "" {
		t.Errorf("SocketPath = %q, expected it to stay empty in demo mode", cfg.SocketPath)
	}
	// The placeholder is replaced rather than kept, which is the other half of
	// why the schema must not treat it as an error.
	if len(cfg.SessionKey) < minSessionKeyLen || cfg.SessionKey == "CHANGE_ME_32_BYTES_HEX_ENCODED_SESSION_SECRET_HERE_XXXXXXXX" {
		t.Errorf("the placeholder session_key was not replaced: %q", cfg.SessionKey)
	}
}
