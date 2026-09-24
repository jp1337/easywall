package web

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/jp1337/easywall/internal/shared"
)

// Every option's four sentences exist in both strict languages. The template
// asks for them through optionHelp's methods, which TestTemplatesOnlyUseTranslatedKeys
// cannot see — its regex reads only T "literal" — so a key missing here would
// render as "opt_drop_multicast_breaks" to the operator and fail nothing else.
//
// They are rendered with plain T, so a markup marker would reach the page as
// a literal backtick or asterisk; TestMarkupStringsAreRenderedThroughRichText
// cannot see these calls either.
func TestOptionHelpsAreComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, lang := range StrictLangs {
		strs := localeStrings(t, lang)
		for _, h := range optionHelps {
			for _, id := range []string{h.DescKey(), h.ProtectsKey(), h.BreaksKey(), h.WhenKey()} {
				text, ok := strs[id]
				if !ok {
					t.Errorf("locales/%s.json has no %q for option %s", lang, id, h.Key)
					continue
				}
				if strings.ContainsAny(text, "`*") || strings.Contains(text, "{}") {
					t.Errorf("locales/%s.json %q carries markup, but the disclosure renders it with plain T: %q", lang, id, text)
				}
			}
		}
	}
	for _, h := range optionHelps {
		if seen[h.Key] {
			t.Errorf("optionHelps lists %s twice", h.Key)
		}
		seen[h.Key] = true
	}
}

// gfmHeadingID is the id kramdown-parser-gfm 1.1.0 gives a heading — the parser
// docs/_config.yml selects (input: GFM) at the version docs/Gemfile.lock pins.
// lib/kramdown/parser/gfm.rb, generate_gfm_header_id: downcase, delete every
// character that is not \p{Word}, '-', space or tab, turn each space and tab
// into '-', and suffix -1, -2 … on a repeat. The raw text it starts from is the
// heading's text with inline markup removed (update_raw_text), so a code span
// keeps its content and loses its backticks.
func gfmHeadingID(heading string, seen map[string]int) string {
	text := strings.NewReplacer("`", "", "*", "").Replace(heading)
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case r == ' ' || r == '\t':
			b.WriteRune('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) ||
			unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Pc, r):
			b.WriteRune(r)
		}
	}
	id := b.String()
	if n := seen[id]; n > 0 {
		seen[id] = n + 1
		return id + "-" + strconv.Itoa(n)
	}
	seen[id] = 1
	return id
}

// Measured, not derived: both ids are what docs/_site/ contains after
// scripts/docs-build.sh. The second is the case the rule exists for — a code
// span, brackets and an em dash, leaving two hyphens.
func TestGFMHeadingIDMatchesTheBuiltSite(t *testing.T) {
	for heading, want := range map[string]string{
		"Which to turn on":                  "which-to-turn-on",
		"`[firewall]` — Protection Modules": "firewall--protection-modules",
	} {
		if got := gfmHeadingID(heading, map[string]int{}); got != want {
			t.Errorf("gfmHeadingID(%q) = %q, the site generates %q", heading, got, want)
		}
	}
	// A repeat takes -1, -2 … (gfm.rb: @id_counter = Hash.new(-1), suffix when > 0).
	seen := map[string]int{}
	if a, b := gfmHeadingID("Logging", seen), gfmHeadingID("Logging", seen); a != "logging" || b != "logging-1" {
		t.Errorf("a repeated heading got %q then %q, want logging then logging-1", a, b)
	}
}

// filtersHeadingIDs is every ATX heading's id in docs/_docs/features/filters.md.
// A "# ..." line inside a fenced block is counted too, which kramdown would not
// do. It adds an id no option links to, and would shift a repeat's suffix only
// if a fenced line repeated a heading, which none on this page does.
func filtersHeadingIDs(t *testing.T) map[string]bool {
	t.Helper()
	path := filepath.Join(filepath.Dir(localesDir(t)), "docs", "_docs", "features", "filters.md")
	raw, err := os.ReadFile(path) // #nosec G304 -- a fixed path in this repository
	if err != nil {
		t.Fatal(err)
	}
	ids, seen := map[string]bool{}, map[string]int{}
	for _, m := range regexp.MustCompile(`(?m)^#{1,6}[ \t]+(.*?)[ \t#]*$`).FindAllStringSubmatch(string(raw), -1) {
		ids[gfmHeadingID(m[1], seen)] = true
	}
	return ids
}

// Every disclosure links to a heading the published page really has. A
// renamed heading leaves a link that opens the page at the top, which looks
// like it worked.
func TestOptionHelpAnchorsAreFiltersHeadings(t *testing.T) {
	ids := filtersHeadingIDs(t)
	if !ids["which-to-turn-on"] {
		t.Fatal("filters.md has no #which-to-turn-on — the parser above or the page changed shape, and options.html links there too")
	}
	for _, h := range optionHelps {
		if !ids[h.Anchor] {
			t.Errorf("option %s links to #%s, which is not a heading in docs/_docs/features/filters.md", h.Key, h.Anchor)
		}
	}
}

var (
	toggleRe = regexp.MustCompile(`name="([a-z_]+)" class="toggle"`)
	cardIDRe = regexp.MustCompile(`^id="opt-([a-z_]+)">`)
	helpRe   = regexp.MustCompile(`\{\{template "opt_help" \(optHelp "([a-z_]+)"\)\}\}`)
)

// Every card on the page is in the table, every entry in the table is a card,
// and each card carries its own id and its own disclosure — not its
// neighbour's, which is what a copy-pasted card would do.
func TestOptionsPageExplainsEveryModule(t *testing.T) {
	path := filepath.Join(filepath.Dir(localesDir(t)), "web", "templates", "options.html")
	raw, err := os.ReadFile(path) // #nosec G304 -- a fixed path in this repository
	if err != nil {
		t.Fatal(err)
	}
	cards := strings.Split(string(raw), `<div class="module" `)[1:]
	if n := strings.Count(string(raw), `class="toggle"`); n != len(optionHelps) {
		t.Errorf("options.html has %d switches, optionHelps %d — a card without id=\"opt-<key>\" is not split out above", n, len(optionHelps))
	}
	if len(cards) != len(optionHelps) {
		t.Errorf("options.html has %d module cards, optionHelps %d", len(cards), len(optionHelps))
	}
	onPage := map[string]bool{}
	for _, card := range cards {
		id, toggle, help := cardIDRe.FindStringSubmatch(card), toggleRe.FindStringSubmatch(card), helpRe.FindStringSubmatch(card)
		if toggle == nil {
			t.Errorf("a module card has no toggle: %.60q", card)
			continue
		}
		key := toggle[1]
		onPage[key] = true
		if id == nil || id[1] != key {
			t.Errorf("the %s card is not id=\"opt-%s\" — /blocked links to that id", key, key)
		}
		if help == nil || help[1] != key {
			t.Errorf("the %s card does not render optHelp %q", key, key)
		} else if strings.Index(card, help[0]) < strings.Index(card, "</label>") {
			t.Errorf("the %s card's disclosure is inside its header <label> — its summary would become part of the switch's name", key)
		}
	}
	for _, h := range optionHelps {
		if !onPage[h.Key] {
			t.Errorf("optionHelps has %s, which no card on /options renders", h.Key)
		}
	}
}

// The page as served, in both strict languages: every card id is there, every
// disclosure rendered, and no raw message id reached the operator.
func TestOptionsPageRendersEveryDisclosure(t *testing.T) {
	for _, lang := range StrictLangs {
		fc := newFakeCore(t)
		s := newTestServer(t, fc)
		enrollFactor(t, s)
		fc.SetResponse(shared.CmdGetOptions, successResp(shared.FirewallOptions{}))

		rec := doRequest(s, http.MethodGet, "/options", nil,
			makeAuthCookie(t, s), &http.Cookie{Name: LangCookie, Value: lang})
		assertStatus(t, rec, http.StatusOK)
		body := rec.Body.String()

		if n := strings.Count(body, `<details class="pkt-detail module-help">`); n != len(optionHelps) {
			t.Errorf("[%s] %d disclosures rendered, want %d", lang, n, len(optionHelps))
		}
		for _, h := range optionHelps {
			if !strings.Contains(body, `id="opt-`+h.Key+`"`) {
				t.Errorf("[%s] no id=\"opt-%s\" on the page", lang, h.Key)
			}
			if !strings.Contains(body, `href="`+h.DocsURL()+`"`) {
				t.Errorf("[%s] the %s card does not link to %s", lang, h.Key, h.DocsURL())
			}
		}
		if raw := regexp.MustCompile(`>\s*(opt|options)_[a-z_]+\s*<`).FindString(body); raw != "" {
			t.Errorf("[%s] a message id reached the page: %s", lang, raw)
		}
	}
}
