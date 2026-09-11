package shared

import "testing"

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
		if !namedOutsideAComment(dockerfile, f.dockerFlag) {
			t.Errorf("the Dockerfile's HEALTHCHECK %s is not %q — and compose still says %q",
				f.name, f.dockerFlag, f.composeKey)
		}
		if !namedOutsideAComment(compose, f.composeKey) {
			t.Errorf("compose's healthcheck %s is not %q — and the Dockerfile still says %q",
				f.name, f.composeKey, f.dockerFlag)
		}
	}

	// The probe itself: same endpoint, same flags, however the two files spell
	// the argument list.
	// namedOutsideAComment throughout, not strings.Contains, for the reason this
	// file has carried since 2.17: both files argue their case in prose, and
	// Step 1 adds a compose comment naming every field above. A Contains check
	// would be satisfied by that comment — "a checker satisfied by the sentence
	// describing the thing it checks". No new import: the helper is package-local.
	for _, fragment := range []string{"--no-check-certificate", "https://127.0.0.1:12227/healthz"} {
		if !namedOutsideAComment(dockerfile, fragment) {
			t.Errorf("the Dockerfile's probe does not contain %q", fragment)
		}
		if !namedOutsideAComment(compose, fragment) {
			t.Errorf("compose's probe does not contain %q", fragment)
		}
	}
}
