package shared

import "testing"

// One definition of the container's health check, and the Dockerfile holds it.
//
// docker-compose.yml carried a `healthcheck:` block for a year, testing
// `test -S /run/easywall/core.sock` and fetching /login while a plain
// `docker run` was checked by nothing at all. 2.17 gave the image a HEALTHCHECK
// and deleted the copy, and the comment left in its place argues the case by
// analogy to release.yml refusing a second packaging definition — two
// definitions of one artefact is how a package comes to contain no binaries.
//
// It then shipped a comment rather than a refusal, which is what this test is.
// build.yml already states the consequence of a block coming back: its
// health-check job measures the image's check and covers compose only
// *because* compose declares none.
//
// Comment lines are skipped, and that is not a nicety. The compose file's own
// comment says "There is deliberately no `healthcheck:` here", and the
// Dockerfile's says why its HEALTHCHECK asks for /healthz alone — so a plain
// strings.Contains over either file would be a checker satisfied by the
// sentence describing the thing it checks. Nine guards in this release were
// green for the wrong reason and six failed exactly that way.
func TestTheContainerHealthCheckHasOneDefinition(t *testing.T) {
	compose := repoFile(t, "docker-compose.yml")
	if namedOutsideAComment(compose, "healthcheck:") {
		t.Error("docker-compose.yml declares a `healthcheck:` block.\n" +
			"  The image's HEALTHCHECK governs this path — compose inherits it when this " +
			"file declares none — so a block here is a second definition of one " +
			"artefact, and the only one of the two that can drift.\n" +
			"  It also silently narrows CI: build.yml's health-check job measures the " +
			"image's check, and covers compose only while compose has no check of its " +
			"own.\n" +
			"  The two cases that legitimately need one — a moved bind_addr, and a " +
			"locally built OCI image on podman — are named in the comment in that file, " +
			"and taking either is a decision to make in review rather than a line to add.")
	}

	dockerfile := repoFile(t, "Dockerfile")
	if !namedOutsideAComment(dockerfile, "HEALTHCHECK") {
		t.Error("the Dockerfile declares no HEALTHCHECK instruction.\n" +
			"  Deleting it leaves both a plain `docker run` and `docker compose up` " +
			"checked by nothing, because compose deliberately declares none of its own. " +
			"That is the state a container was in when it stayed \"Up\" for hours with a " +
			"live core and a dead web process.")
	}
}
