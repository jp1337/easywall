package shared

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// selftestUnit is the one unit that is allowed to hold CAP_SYS_ADMIN.
const selftestUnit = "easywall-selftest.service"

// unitFiles lists systemd/*.service, so a unit added later is covered without
// anybody remembering to add it here. A glob that matched nothing would make
// every assertion below vacuous, so an empty result is a failure.
func unitFiles(t *testing.T) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(repoRoot(t), "systemd", "*.service"))
	if err != nil {
		t.Fatalf("glob systemd/*.service: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no unit files found under systemd/: this guard is reading nothing " +
			"and would pass however the units are written")
	}
	units := make(map[string]string, len(paths))
	for _, p := range paths {
		b, err := os.ReadFile(p) // #nosec G304 -- a path this test globbed itself
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		units[filepath.Base(p)] = string(b)
	}
	return units
}

// namedOutsideAComment reports whether needle appears on a line that is not a
// comment, in any of the syntaxes this file reads: unit files, shell, make, YAML
// and Go.
//
// The distinction is not pedantry, it is the difference between a guard and a
// tautology, and both guards below were written wrong first and caught by
// mutation. easywall-core.service's longest comment is the one explaining why it
// does *not* take CAP_SYS_ADMIN, so a plain strings.Contains flagged the unit
// for documenting the rule. And test.yml's comment explains what
// EASYWALL_REQUIRE_SELFTEST is for, so deleting the assignment that actually
// sets it left the check green. Every guard in this file is verified by
// deleting the line it is about and watching it go red; two of them did not,
// until this function existed.
func namedOutsideAComment(body, needle string) bool {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if strings.Contains(line, needle) {
			return true
		}
	}
	return false
}

// CAP_SYS_ADMIN is granted by exactly one unit, and that unit is named here.
//
// The whole shape of 2.17's layer C rests on this. The self-test builds a
// throwaway network namespace to measure the rules easywall wrote against a real
// packet, CLONE_NEWNET requires CAP_SYS_ADMIN, and easywall-core.service bounds
// the long-lived root daemon to CAP_NET_ADMIN and says "nothing else is needed".
// Widening that daemon — the one that holds the control socket and answers a
// network-facing process — to buy a check that runs once per upgrade is the
// trade this repository declined. So the proof got a Type=oneshot unit that is
// alive for milliseconds, holds no socket and listens on nothing.
//
// Adding CAP_SYS_ADMIN to easywall-core.service would be a one-line diff that
// makes the self-test work everywhere and looks like a fix. This is the test
// that says no.
func TestCapSysAdminIsGrantedByExactlyOneUnit(t *testing.T) {
	units := unitFiles(t)

	if _, ok := units[selftestUnit]; !ok {
		t.Fatalf("systemd/%s is missing; it is the unit that carries the "+
			"self-test's CAP_SYS_ADMIN, and without it the proof cannot run on a "+
			"packaged installation at all", selftestUnit)
	}

	for name, body := range units {
		holds := namedOutsideAComment(body, "CAP_SYS_ADMIN")
		switch {
		case name == selftestUnit && !holds:
			t.Errorf("systemd/%s no longer grants CAP_SYS_ADMIN — CLONE_NEWNET needs it, "+
				"so the self-test would record \"unprovable\" on every installation and "+
				"this release would prove nothing where it matters", name)
		case name != selftestUnit && holds:
			t.Errorf("systemd/%s grants CAP_SYS_ADMIN, and only %s may.\n"+
				"  CAP_SYS_ADMIN on a long-lived root daemon that holds the control "+
				"socket is not a capability this repository spends to make a "+
				"once-per-upgrade check convenient. Read the comment above "+
				"AmbientCapabilities= in systemd/easywall-core.service.", name, selftestUnit)
		}
	}
}

// A unit file that nothing installs is a unit that does not exist on a real
// machine. This is the class of mistake that shipped a package with no binaries
// at all for the whole life of it — dh_prep emptied debian/easywall between the
// build and the assembly, CI built the artefact without ever installing it, and
// systemd answered 203/EXEC every five seconds on every host that had it. See
// docs-tech/packaging.md.
//
// So the check is driven by the directory listing rather than by a list somebody
// maintains: every systemd/*.service has to be installed by debian/rules and by
// the Makefile's install target, and has to be enabled by postinst and disabled
// by prerm. Adding a unit and forgetting one of those four is otherwise
// completely silent.
//
// **Four, and not five.** This comment said "enabled, started, stopped and
// disabled" while the table below held four rows, and the review measured what
// that cost: deleting the `systemctl stop` line from debian/prerm leaves this
// suite green. `systemctl start` is not checked here either — build.yml's
// install-verify job catches that half with `is-active`, and nothing at all
// checks the stop. A guard described as covering more than it does is this
// release's own subject, so the description is corrected rather than the table
// quietly extended: adding a row is a change to what `make test` demands of
// the maintainer scripts, and belongs in a change of its own.
func TestEveryUnitIsInstalledAndManaged(t *testing.T) {
	root := repoRoot(t)

	// Each file, the verb the unit's name has to appear next to, and what its
	// absence would mean on a real host.
	//
	// The verb is not decoration. The first version of this test asked only
	// whether the file contained the unit's name, and it stayed green through a
	// mutation that deleted the install line from debian/rules — because the
	// comment above that line still said "easywall-selftest.service". A guard
	// satisfied by the sentence describing the thing it is checking is exactly
	// the shape of test 2.12 found seven of, and it was found here the same way
	// they were: by breaking the implementation and watching nothing happen.
	for _, f := range []struct {
		path        string
		verb        string
		consequence string
	}{
		{filepath.Join("debian", "rules"), "install",
			"the .deb would not contain the unit at all"},
		{filepath.Join("debian", "postinst"), "systemctl enable",
			"the unit would be on disk and never enabled, so it would not run at boot"},
		{filepath.Join("debian", "prerm"), "systemctl disable",
			"removing the package would leave an enabled unit pointing at a deleted binary"},
		{"Makefile", "install",
			"`make install` would produce a host missing the unit the package installs"},
	} {
		b, err := os.ReadFile(filepath.Join(root, f.path)) // #nosec G304 -- a fixed list of repository paths
		if err != nil {
			t.Fatalf("read %s: %v", f.path, err)
		}
		for name := range unitFiles(t) {
			if !actedOn(string(b), name, f.verb) {
				t.Errorf("no %q line in %s names %s: %s", f.verb, f.path, name, f.consequence)
			}
		}
	}
}

// The CI gate that makes a skipped proof a failure is spelled out in two files,
// and this is what keeps the two spellings the same.
//
// internal/core/selftest_required_test.go reads EASYWALL_REQUIRE_SELFTEST and
// turns every "the harness cannot be built here" skip into a t.Fatal;
// .github/workflows/test.yml sets it on the integration step. Renaming it in one
// place and not the other disables the gate completely, and nothing else would
// notice: the suite would go back to skipping politely and the tick would stay
// green — which is the exact failure the gate exists to catch, one level further
// out. It is checked from this package rather than from the integration suite
// because a guard behind the `integration` build tag is not run by `make test`.
func TestTheCIProofGateIsStillWiredUp(t *testing.T) {
	const gate = "EASYWALL_REQUIRE_SELFTEST"
	root := repoRoot(t)

	// The needle is anchored per file, and the two characters doing the
	// anchoring are the whole test.
	//
	// The first version asked namedOutsideAComment for the bare name, which is a
	// strings.Contains — so any rename that *contains* the old one passed on both
	// sides while the gate was already dead. EASYWALL_REQUIRE_SELFTESTS and
	// EASYWALL_REQUIRE_SELFTEST_STRICT both left it green, and suffixing is the
	// most likely rename there is. That made this the seventh test in this
	// release to be green for the wrong reason, and it was inside the helper
	// written to close the sixth: the fix for a substring guard was another
	// substring guard.
	//
	// A closing quote in Go and an `=` in the workflow are both a character the
	// name cannot grow through. They also close a case namedOutsideAComment
	// structurally cannot see, because it reads whole lines: a trailing comment
	// that merely mentions the variable, `run: sudo go test … # EASYWALL_...
	// removed`, is no longer enough to satisfy the check.
	for _, f := range []struct {
		path   string
		anchor string
		what   string
	}{
		{filepath.Join("internal", "core", "selftest_required_test.go"),
			`"` + gate + `"`,
			"the test helper that turns an unprovable self-test into a failure"},
		{filepath.Join(".github", "workflows", "test.yml"),
			gate + "=",
			"the workflow step that sets it"},
	} {
		b, err := os.ReadFile(filepath.Join(root, f.path)) // #nosec G304 -- a fixed list of repository paths
		if err != nil {
			t.Fatalf("read %s: %v", f.path, err)
		}
		if !namedOutsideAComment(string(b), f.anchor) {
			t.Errorf("%s has no %s outside a comment — %s no longer names the gate.\n"+
				"  Both halves have to spell it identically, or the integration suite "+
				"goes back to skipping the proof and CI reports success for a run that "+
				"measured nothing. A rename has to be made in both places.",
				f.path, f.anchor, f.what)
		}
	}
}

// actedOn reports whether some line that is not a comment carries both the unit
// name and the verb that acts on it.
func actedOn(body, unit, verb string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if strings.Contains(line, unit) && strings.Contains(line, verb) {
			return true
		}
	}
	return false
}
