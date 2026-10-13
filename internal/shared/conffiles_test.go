package shared

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A configuration file easywall rewrites must not be shipped under its own name.
//
// debhelper marks everything a package installs under /etc as a dpkg conffile,
// and dpkg then treats a file the program edits as one the *admin* edited. Both
// of easywall's TOMLs are rewritten while it runs — the settings pages write
// easywall.toml through the core, the wizard and the password page write
// web.toml — so both have to arrive as templates that postinst copies into
// place, which is what keeps them out of the conffile list.
//
// web.toml always did. easywall.toml did not, and the cost was measured by
// upgrading 2.5.1 to 2.5.2 in a debian:trixie container with `apt-get install -y`
// and one setting saved beforehand:
//
//	Configuration file '/etc/easywall/easywall.toml'
//	 ==> Modified (by you or by a script) since installation.
//	 end of file on stdin at conffile prompt
//	dpkg-query: install ok unpacked 2.5.2
//
// "unpacked", not "installed": dpkg stopped at the prompt, so postinst never
// ran and the services were never restarted — new binaries on disk, old
// processes serving. Any unattended upgrade that does not pass --force-confold
// takes that path, and a package cannot assume the administrator's does.
func TestNeitherConfigIsShippedAsAConffile(t *testing.T) {
	root := repoRoot(t)

	rules, err := os.ReadFile(filepath.Join(root, "debian", "rules"))
	if err != nil {
		t.Fatalf("read debian/rules: %v", err)
	}
	postinst, err := os.ReadFile(filepath.Join(root, "debian", "postinst"))
	if err != nil {
		t.Fatalf("read debian/postinst: %v", err)
	}

	for _, name := range []string{"easywall.toml", "web.toml"} {
		// Installed into /etc under its own name, with nothing after it, is what
		// makes it a conffile.
		direct := regexp.MustCompile(
			`etc/easywall/` + regexp.QuoteMeta(name) + `(\s|$)`)
		for _, line := range strings.Split(string(rules), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
			if direct.MatchString(line) {
				t.Errorf("debian/rules installs %s into /etc under its own name:\n  %s\n"+
					"  easywall rewrites that file, so dpkg would track it as a conffile a "+
					"script modifies — and an unattended upgrade stops at the prompt with "+
					"the package left unconfigured. Install it as %s.template and copy it "+
					"in postinst, the way web.toml already is.", name, strings.TrimSpace(line), name)
			}
		}

		// And the template has to actually be turned into the real file, or a
		// fresh install has no configuration at all.
		if !strings.Contains(string(rules), name+".template") {
			t.Errorf("debian/rules does not install %s.template", name)
		}
		if !strings.Contains(string(postinst), name+".template") {
			t.Errorf("debian/postinst never creates /etc/easywall/%s from its template, "+
				"so a fresh install would have no %s", name, name)
		}
	}
}

// dh_fixperms resets every file in the package to 0644, or 0755 if it is
// executable, and debian/rules does not override it — so an `install -m` with
// any other mode states a permission the .deb does not have. The two config
// templates said 0640 and shipped 0644, measured with dpkg-deb -c.
func TestDebianRulesModesAreWhatShips(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "debian", "rules"))
	if err != nil {
		t.Fatalf("read debian/rules: %v", err)
	}
	if strings.Contains(string(b), "override_dh_fixperms") {
		t.Skip("debian/rules overrides dh_fixperms; the modes it states may hold")
	}
	modes := regexp.MustCompile(`install\s+-m\s*(\d+)`).FindAllStringSubmatch(string(b), -1)
	if len(modes) == 0 {
		t.Fatal("no `install -m` in debian/rules: this guard is reading nothing")
	}
	for _, m := range modes {
		if m[1] != "0644" && m[1] != "0755" {
			t.Errorf("debian/rules installs a file -m %s; dh_fixperms makes it 0644 or 0755, "+
				"so the package does not carry the mode this line states", m[1])
		}
	}
}
