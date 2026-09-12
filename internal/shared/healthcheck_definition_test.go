package shared

import (
	"strings"
	"testing"
)

// namedOutsideACommentWithBoundary is namedOutsideAComment plus a check that
// the character right after the match, if any, is not a digit.
//
// namedOutsideAComment alone is satisfied by more than an exact value:
// "--retries=3" is a prefix of "--retries=30", and "retries: 3" the same for
// compose — so a changed retry count still passed this test. interval,
// timeout and start_period do not have this hazard today, each ending in a
// unit suffix ("10s", "5s", "15s") that a differing digit count already
// breaks on its own, but every field in the loop below goes through this to
// stop that from being an accident of which fields happen to end in a
// letter rather than a property this test enforces.
//
// Scoped to this file rather than folded into namedOutsideAComment itself:
// that helper is shared with guards elsewhere (systemd units, CI workflow
// wiring) whose needles are not bare values with a digit boundary to worry
// about, and tightening it there is a change to tests this one has no
// business making.
func namedOutsideACommentWithBoundary(body, needle string) bool {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		for i := 0; ; {
			j := strings.Index(line[i:], needle)
			if j < 0 {
				break
			}
			at := i + j
			after := at + len(needle)
			if after >= len(line) || line[after] < '0' || line[after] > '9' {
				return true
			}
			i = after
		}
	}
	return false
}

// TestTheContainerHealthCheckHasOneDefinition asserts the image's HEALTHCHECK
// and compose's healthcheck describe the same check.
//
// It used to forbid the compose block outright, and that was right until it met
// podman: OCI is podman's default image format and has no healthcheck field, so
// `podman build` exits 0 with HealthCheck: null, and docker-compose.yml carries
// a build: section while the documented path is `docker compose up -d`. An
// operator following the documentation got no check and no error. That case was
// named in this test's own old message as one requiring a decision in review;
// 2.19 took it.
//
// So there are two definitions now, and the property that matters is not that
// there is one — it is that they do not drift. Every field is compared. The
// name is unchanged because invariants.md and the compose comment both point
// at it.
func TestTheContainerHealthCheckHasOneDefinition(t *testing.T) {
	dockerfile := repoFile(t, "Dockerfile")
	compose := repoFile(t, "docker-compose.yml")

	if !namedOutsideAComment(dockerfile, "HEALTHCHECK") {
		t.Fatal("the Dockerfile declares no HEALTHCHECK instruction.\n" +
			"  A plain `docker run` is then checked by nothing. That is the state a " +
			"container was in when it stayed \"Up\" for hours with a live core and a dead " +
			"web process.")
	}
	if !namedOutsideAComment(compose, "healthcheck:") {
		t.Fatal("docker-compose.yml declares no `healthcheck:` block.\n" +
			"  It needs one: podman's default OCI format has no healthcheck field, so a " +
			"locally built image carries none and `podman build` exits 0 anyway. Compose " +
			"inheriting the image's check only works where the image has one.")
	}

	for _, f := range []struct{ name, dockerFlag, composeKey string }{
		{"interval", "--interval=10s", "interval: 10s"},
		{"timeout", "--timeout=5s", "timeout: 5s"},
		{"start period", "--start-period=15s", "start_period: 15s"},
		{"retries", "--retries=3", "retries: 3"},
	} {
		if !namedOutsideACommentWithBoundary(dockerfile, f.dockerFlag) {
			t.Errorf("the Dockerfile's HEALTHCHECK %s is not %q — and compose still says %q",
				f.name, f.dockerFlag, f.composeKey)
		}
		if !namedOutsideACommentWithBoundary(compose, f.composeKey) {
			t.Errorf("compose's healthcheck %s is not %q — and the Dockerfile still says %q",
				f.name, f.composeKey, f.dockerFlag)
		}
	}

	// The probe itself: same endpoint, same flags, however the two files spell
	// the argument list.
	// namedOutsideAComment, not strings.Contains, for the reason this file has
	// carried since 2.17: both files argue their case in prose, and Step 1
	// adds a compose comment naming every field above. A Contains check would
	// be satisfied by that comment — "a checker satisfied by the sentence
	// describing the thing it checks". No digit-boundary hazard here — both
	// fragments are long enough that a differing value cannot leave one a
	// prefix of the other — so the plain helper is enough.
	for _, fragment := range []string{"--no-check-certificate", "https://127.0.0.1:12227/healthz"} {
		if !namedOutsideAComment(dockerfile, fragment) {
			t.Errorf("the Dockerfile's probe does not contain %q", fragment)
		}
		if !namedOutsideAComment(compose, fragment) {
			t.Errorf("compose's probe does not contain %q", fragment)
		}
	}
}
