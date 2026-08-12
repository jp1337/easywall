package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jp1337/easywall/config"
	"github.com/jp1337/easywall/internal/shared"
)

// The same round trip as the core's: what --write-config writes must load.
//
// web.toml has one extra property worth asserting. It ships with the session
// key left as a published placeholder, and easywall replaces that on first
// start precisely because a firewall interface signing cookies with a key
// anyone can read from this repository can be signed into by anyone. A default
// that arrived with a *plausible-looking* key nobody replaced would be worse
// than the placeholder.
func TestWriteConfigProducesSomethingTheWebProcessCanLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.toml")

	if err := shared.WriteDefaultConfig(path, config.Web); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Before loading: the file carries the obvious placeholder rather than a
	// key that looks real and is published in this repository.
	written, err := os.ReadFile(path) // #nosec G304 -- a path this test just created
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "CHANGE_ME") {
		t.Error("the written default does not carry the placeholder session key; " +
			"a plausible-looking key nobody replaces is worse than an obvious one")
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("the embedded default does not load: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the embedded default does not validate: %v", err)
	}

	// And loading it replaces that placeholder immediately — this is the first
	// start easywall does on every fresh installation, and the reason the key
	// being published costs nothing.
	if strings.Contains(cfg.SessionKey, "CHANGE_ME") || len(cfg.SessionKey) != 64 {
		t.Errorf("after loading, session_key is %q; the placeholder should have been "+
			"replaced by a generated 32-byte key", cfg.SessionKey)
	}

	if cfg.Username != "" || cfg.Password != "" {
		t.Error("the default ships with credentials; the first-run wizard sets them")
	}
	if cfg.DemoMode {
		t.Error("the default ships with demo mode on — that reaches no firewall at all")
	}
}
