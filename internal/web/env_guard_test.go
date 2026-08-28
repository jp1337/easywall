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

// The two tests above are both hand-written lists checked against each other,
// which catches a key falling out of the accounting entirely but not a key
// moved between the two categories in the same change: drop a key from
// secretManagedKeys and add a matching entry to shared.WebEnvVars in one
// commit, and both tests above still pass, each reading the other's half of
// the same mistake as the reason it is fine.
//
// telemetry is the only managed key precedence lets a variable name (see the
// header comment on shared.WebEnvVars), so the intersection of managedKeys
// and the keys shared.WebEnvVars targets has exactly one member. A second
// member means a secret was reclassified as env-settable, or the reverse,
// without anyone deciding it belonged in this test's exception.
func TestOnlyTelemetryIsBothManagedAndEnvSettable(t *testing.T) {
	managed := map[string]bool{}
	for _, k := range managedKeys {
		managed[k] = true
	}
	var both []string
	for _, v := range shared.WebEnvVars {
		if managed[v.TOMLKey] {
			both = append(both, v.TOMLKey)
		}
	}
	if len(both) != 1 || both[0] != "telemetry" {
		t.Errorf("managed keys named by an environment variable = %v, want exactly [telemetry]", both)
	}
}
