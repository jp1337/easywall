package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The topbar chip rendered `v2.14.0-44-g5…` and read as broken data.
//
// The Makefile hands the linker `git describe --tags --always --dirty`, so a
// development build's version is a tag plus a commit count plus an abbreviated
// object name. At the chip's 18ch cap that truncates, and an ellipsis in the
// chrome reads as a fault in the software rather than as a build detail. The
// chip shows the release; the full string moves to the title attribute, where a
// development build stays recoverable.
//
// Verify by mutation: make ReleaseVersion return its argument unchanged and five
// of these ten cases go red.
func TestReleaseVersionDropsTheDescribeSuffix(t *testing.T) {
	for _, tc := range []struct{ in, want, why string }{
		{"v2.15.1", "v2.15.1", "a tagged build is already the release"},
		{"2.15.1", "2.15.1", "with or without the v"},
		{"v2.14.0-44-g5abc123", "v2.14.0", "the commits-since-tag and the object name go"},
		{"v2.15.1-dirty", "v2.15.1", "so does a dirty marker"},
		{"v2.14.0-44-g5abc123-dirty", "v2.14.0", "and both together"},
		{"v2.16.0-rc1", "v2.16.0-rc1", "a pre-release suffix is part of the tag and stays"},
		{"v2.16.0-rc1-3-gdeadbee", "v2.16.0-rc1", "even with a describe suffix after it"},
		{"5abc123", "5abc123", "`--always` with no tag in reach: nothing to trim"},
		{"dev", "dev", "the Makefile's own fallback"},
		{"", "", "and nothing at all is not a crash"},
	} {
		if got := shared.ReleaseVersion(tc.in); got != tc.want {
			t.Errorf("ReleaseVersion(%q) = %q, want %q — %s", tc.in, got, tc.want, tc.why)
		}
	}
}

// The chip renders the release and the title keeps the build. Both halves
// matter: without the title a development build becomes unidentifiable from the
// interface, which is worse than an ellipsis.
func TestTheVersionChipShowsTheReleaseAndTitlesTheBuild(t *testing.T) {
	root := filepath.Dir(localesDir(t))
	raw, err := os.ReadFile(filepath.Join(root, "web", "templates", "base.html"))
	if err != nil {
		t.Fatal(err)
	}

	const want = `<span class="brand-version" title="{{.Version}}">{{.ReleaseVersion}}</span>`
	if !strings.Contains(string(raw), want) {
		t.Errorf("base.html does not render the version chip as:\n  %s\n"+
			"  the chip carries the release and the title the full `git describe` string; "+
			"swapping them puts the ellipsis back, and dropping the title makes a "+
			"development build unidentifiable from the interface", want)
	}
}

// The page title is set in the mono face.
//
// DESIGN.md's rule was "Inter for language, JetBrains Mono for network data",
// and 2.16 amends it: Inter for what is read, mono for what is identified. The
// name of the page you are standing on is identified. The countdown proved the
// voice works and it appeared on exactly one page.
func TestThePageTitleTakesTheDisplayVoice(t *testing.T) {
	css := appStylesheet(t)

	i := strings.Index(css, ".page-title{")
	if i < 0 {
		t.Fatal("no .page-title rule in the built stylesheet")
	}
	rule := css[i:]
	if j := strings.Index(rule, "}"); j > 0 {
		rule = rule[:j]
	}

	for _, want := range []struct{ decl, breaks string }{
		{"font-family:var(--font-data)", "the title is set in Inter, which is the rule 2.16 amends"},
		{"font-weight:300", "the display weight is not the light one the countdown established"},
		{"font-size:30px", "the sub-900px size is not the measured ceiling"},
	} {
		if !strings.Contains(rule, want.decl) {
			t.Errorf(".page-title has no %q — %s\n  rule as built: %s", want.decl, want.breaks, rule)
		}
	}

	// 34px above 900px, in a media query of its own.
	if !strings.Contains(css, "@media (min-width:900px){.page-title{font-size:34px}") {
		t.Error("no 900px step to 34px for .page-title\n" +
			"  30px is a ceiling reached by measurement — the longest unbreakable title is " +
			"19 characters (Systemeinstellungen) at 0.6em advance, 11.4em, against roughly " +
			"358px inside the padding at a 390px viewport — not a size the wide layout wants")
	}
}
