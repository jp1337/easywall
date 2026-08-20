package web

import (
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The other half of the rule: no variable may name one of the six keys the
// interface writes back through mergeConfig. Four of them are secrets, and an
// environment variable is visible in `docker inspect` and /proc/<pid>/environ.
func TestNoEnvVarTargetsAManagedKey(t *testing.T) {
	managed := map[string]bool{}
	for _, k := range managedKeys {
		managed[k] = true
	}
	if len(managed) == 0 {
		t.Fatal("managedKeys is empty; this test would pass for the wrong reason")
	}
	for _, v := range shared.WebEnvVars {
		if managed[v.TOMLKey] {
			t.Errorf("%s targets %q, which the interface writes and saveLocked "+
				"persists — and which is a secret or a consent flag", v.Name, v.TOMLKey)
		}
	}
}
