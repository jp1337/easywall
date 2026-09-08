package web

import (
	"regexp"
	"strings"
	"testing"
)

// The application sidebar gets the divider and the indent the documentation
// sidebar was given, for the reason DESIGN.md already states: a
// nav-section-label at ink-subtle and a nav link at ink-muted sit twelve steps
// apart per channel — distinct on paper, the same grey once rendered, and with
// the label the *lighter* of the two it read as the weaker element rather than
// as the divider it is. After 2.16 there is even less colour to carry it.
//
// carried-forward.md recorded this as "deliberately not touched" across three
// design reviews. It is the entry 2.16 exists to close.
//
// Selector-shaped rather than markup-shaped on purpose: the rules hang off
// :has(> .nav-section-label), so a fourth nav group added to base.html gets the
// divider and the indent without anyone remembering to add a class.
func TestTheSidebarGroupsCarryADividerAndAnIndent(t *testing.T) {
	css := appStylesheet(t)

	for _, want := range []struct{ needle, breaks string }{
		{".nav-section:has(>.nav-section-label)~.nav-section:has(>.nav-section-label)",
			"a labelled group that follows another labelled group has no rule above it, " +
				"so Rules and System run together as one column of equal-weight text"},
		{"padding-left:22px",
			"the links under a label are not indented, so the eye reads two rows of " +
				"equal-weight text instead of container-then-contents"},
	} {
		if !strings.Contains(css, want.needle) {
			t.Errorf("web/static/style.css has no %q\n  without it: %s\n"+
				"  rebuild with `npm run build:css` — Tailwind drops rules silently",
				want.needle, want.breaks)
		}
	}

	// The FIRST labelled group must not get one: it follows the ungrouped
	// Dashboard link, not another group, and a rule there divides nothing. The
	// documentation sidebar says the same thing with :first-of-type. A bare
	// `.nav-section:has(...)` carrying a border-top is the mistake to catch.
	//
	// Anchored on the start of a rule, and it has to be: the correct sibling
	// selector ENDS in the same characters —
	// `…)~.nav-section:has(>.nav-section-label){border-top` — so a plain
	// substring search matches the very rule it is meant to bless and this guard
	// failed against a stylesheet that was right. Found by running it.
	bare := regexp.MustCompile(`(\A|[}])\.nav-section:has\(>\.nav-section-label\)\{border-top`)
	if bare.MatchString(css) {
		t.Error("every labelled group carries a border-top, including the first\n" +
			"  it follows the ungrouped Dashboard link, and a rule there divides nothing")
	}
}

// The active nav item is never marked by a background alone.
//
// select-fill is 1.08:1 against surface in both themes — the same invisibility
// as the focus ring this release replaced, measured at 1.31:1. The edge says
// "this one is active"; the fill only confirms it.
func TestTheActiveNavItemCarriesAnEdgeAndNotOnlyAFill(t *testing.T) {
	css := appStylesheet(t)

	i := strings.Index(css, ".nav-links li.active a:before")
	if i < 0 {
		t.Fatal("the active nav item has no ::before rule in the built stylesheet — " +
			"the edge is gone, and a fill at 1.08:1 against its surface is not a marker")
	}
	rule := css[i:]
	if j := strings.Index(rule, "}"); j > 0 {
		rule = rule[:j]
	}
	if !strings.Contains(rule, "var(--color-select-edge)") {
		t.Errorf("the active item's edge is not select-edge: %s", rule)
	}
}

// The language select is the only native control in the interface, and on the
// login card it was recognisable only by its native arrow. DESIGN.md names the
// failure itself: a field's canvas background sits 1.03–1.06:1 from the surface
// of the card it is on.
func TestTheLanguageSelectIsDrawnLikeEveryOtherControl(t *testing.T) {
	css := appStylesheet(t)

	i := strings.Index(css, ".lang-select{")
	if i < 0 {
		t.Fatal("no .lang-select rule in the built stylesheet")
	}
	rule := css[i:]
	if j := strings.Index(rule, "}"); j > 0 {
		rule = rule[:j]
	}

	for _, want := range []struct{ decl, breaks string }{
		{"appearance:none", "the browser draws its own control and the interface has one thing in it that is not its own"},
		{"var(--color-surface-raised)", "the fill stays canvas, which is 1.03–1.06:1 from the card behind it — on the login card the control has no body of its own"},
	} {
		if !strings.Contains(rule, want.decl) {
			t.Errorf(".lang-select has no %q — %s\n  rule as built: %s", want.decl, want.breaks, rule)
		}
	}

	// appearance:none removes the native arrow, so something has to replace it
	// or the control stops announcing that it opens.
	if !strings.Contains(css, ".lang-switch:after") {
		t.Error("no chevron on .lang-switch — appearance:none took the native arrow " +
			"and nothing put one back, so the select no longer reads as a select")
	}
}
