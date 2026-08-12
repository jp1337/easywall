package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jp1337/easywall/config"
	"github.com/jp1337/easywall/internal/shared"
)

// What --write-config produces has to be a configuration this daemon can start
// from. A template that writes out and then fails to load is worse than no
// template at all: the operator followed the documentation and got a daemon
// that will not come up.
//
// This is the round trip the flag actually performs — embed, write, load,
// validate — rather than a check that two files match.
func TestWriteConfigProducesSomethingTheCoreCanLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "easywall.toml")

	if err := shared.WriteDefaultConfig(path, config.Core); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("the embedded default does not load: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the embedded default does not validate: %v", err)
	}

	// And it is the commented file, not a struct dumped back out. The comments
	// are the reason an operator is told to produce this rather than to write
	// twelve keys from the reference page.
	raw, err := os.ReadFile(path) // #nosec G304 -- a path this test just created
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 || raw[0] != '#' {
		t.Errorf("the written file does not start with a comment; --write-config "+
			"exists to produce the *commented* default, got: %.40q", raw)
	}
}
