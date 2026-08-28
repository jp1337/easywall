package web

import (
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// secretManagedKeys are the managed keys no environment variable may name. Not
// managedKeys itself since 2.12: telemetry is deliberately settable from the
// environment, because the public demo is configured that way and has to report
// like any other installation, and the precedence inversion means an operator's
// answer in the interface beats the variable rather than the other way round.
//
// The five below are different in kind. Four are secrets and one is the hash
// they authenticate against, and an environment variable is not a place for
// either: it is visible in `docker inspect`, to anything reading
// /proc/<pid>/environ, and in whatever log somebody pastes into an issue.
// web.toml is 0600; the environment of a running container is not.
var secretManagedKeys = []string{
	"session_key", "username", "password", "totp_secret", "recovery_codes",
}

func TestNoEnvVarTargetsAManagedKey(t *testing.T) {
	forbidden := map[string]bool{}
	for _, k := range secretManagedKeys {
		forbidden[k] = true
	}
	for _, v := range shared.WebEnvVars {
		if forbidden[v.TOMLKey] {
			t.Errorf("%s targets %q, which is a secret or the hash it authenticates "+
				"against — and an environment variable is readable by anything that "+
				"can read the process", v.Name, v.TOMLKey)
		}
	}
}

// secretManagedKeys is a hand-written subset of managedKeys, so it can fall
// behind the list it subsets. A managed key that is in neither the subset nor
// the environment table is a key nobody has decided about.
func TestEveryManagedKeyIsEitherSecretOrDeliberatelyEnvSettable(t *testing.T) {
	secret := map[string]bool{}
	for _, k := range secretManagedKeys {
		secret[k] = true
	}
	envSettable := map[string]bool{}
	for _, v := range shared.WebEnvVars {
		envSettable[v.TOMLKey] = true
	}
	for _, k := range managedKeys {
		if !secret[k] && !envSettable[k] {
			t.Errorf("managed key %q is neither in secretManagedKeys nor in "+
				"shared.WebEnvVars; decide which it is and say so there", k)
		}
	}
}
