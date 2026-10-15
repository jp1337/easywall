package shared

import (
	"slices"
	"strings"
	"testing"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// countryCodes226 is CountryCodes as 2.26 shipped it: ipverse's 238 country
// directories, `gh api repos/ipverse/country-ip-blocks/contents/country` on
// 2026-10-13. A later release may add codes, never remove one.
const countryCodes226 = `
ad ae af ag ai al am ao aq ar as at au aw ax az ba bb bd be bf bg bh bi
bj bl bm bn bo bq br bs bt bw by bz ca cd cf cg ch ci ck cl cm cn co cr
cu cv cw cy cz de dj dk dm do dz ec ee eg er es et fi fj fk fm fo fr ga
gb gd ge gf gg gh gi gl gm gn gp gq gr gt gu gw gy hk hn hr ht hu id ie
il im in io iq ir is it je jm jo jp ke kg kh ki km kn kp kr kw ky kz la
lb lc li lk lr ls lt lu lv ly ma mc md me mf mg mh mk ml mm mn mo mp mq
mr ms mt mu mv mw mx my mz na nc ne nf ng ni nl no np nr nu nz om pa pe
pf pg ph pk pl pm pr ps pt pw py qa re ro rs ru rw sa sb sc sd se sg si
sk sl sm sn so sr ss st sv sx sy sz tc td tg th tj tk tl tm tn to tr tt
tv tw tz ua ug us uy uz va vc ve vg vi vn vu wf ws ye yt za zm zw
`

// Spec D6. Every code is a country x/text knows by name in the three
// languages the interface ships — the select renders that name — and is
// spelled the way the kernel set name and log prefix need it. ParseRegion
// alone proves little: it accepts "zz" (unknown region), "eu" (a grouping)
// and "uk" (an alias of GB); IsCountry and Canonicalize refuse those.
func TestEveryCountryCodeIsACountry(t *testing.T) {
	if len(CountryCodes) < 238 {
		t.Fatalf("%d codes; ipverse publishes 238", len(CountryCodes))
	}
	seen := map[string]bool{}
	for _, cc := range CountryCodes {
		if seen[cc] {
			t.Errorf("%q is listed twice", cc)
		}
		seen[cc] = true
		r, err := language.ParseRegion(cc)
		if err != nil || !r.IsCountry() || r.Canonicalize() != r || strings.ToLower(r.String()) != cc {
			t.Errorf("%q is not a canonical ISO 3166-1 country code (%v, %v)", cc, r, err)
			continue
		}
		for _, lang := range []string{"en", "de", "fr"} {
			if display.Regions(language.Make(lang)).Name(r) == "" {
				t.Errorf("%q has no %s name", cc, lang)
			}
		}
		id := CountryFeedID(cc)
		if got, ok := CountryCode(id); !ok || got != cc || !KnownFeedID(id) {
			t.Errorf("%s does not read back as %q", id, cc)
		}
		if _, ok := CatalogueFeedByID(id); !ok {
			t.Errorf("CatalogueFeedByID(%s) found nothing", id)
		}
	}
	for _, f := range FeedCatalogue {
		if IsCountryFeedID(f.ID) || strings.HasPrefix(f.ID, countryFeedPrefix) {
			t.Errorf("catalogue id %q is spelled like a country's", f.ID)
		}
	}
}

// invariants.md: a catalogue id is retired, never deleted. A code dropped
// from CountryCodes makes ValidateRules refuse every rule set that has it on,
// and the core refuses to boot with it.
func TestCountryCodesAreNeverRemoved(t *testing.T) {
	for _, cc := range strings.Fields(countryCodes226) {
		if !slices.Contains(CountryCodes, cc) {
			t.Errorf("%q was in 2.26 and is gone", cc)
		}
	}
}

func TestCountryFeedIDs(t *testing.T) {
	for id, want := range map[string]bool{
		"country-de": true, "country-us": true, "country-aq": true,
		"country-zz": false, "country-DE": false, "country-": false, "country-deu": false,
		"country-uk": false, "country-eu": false, "de": false, "country-de ": false,
	} {
		if got := IsCountryFeedID(id); got != want {
			t.Errorf("IsCountryFeedID(%q) = %v, want %v", id, got, want)
		}
		if got := KnownFeedID(id); got != want {
			t.Errorf("KnownFeedID(%q) = %v, want %v", id, got, want)
		}
	}
	f, ok := CatalogueFeedByID("country-de")
	if !ok || f.Name != "DE" || f.Format != FeedFormatIPverse || f.Interval != CountryInterval ||
		!slices.Equal(f.URLs, []string{"https://raw.githubusercontent.com/ipverse/country-ip-blocks/master/country/de/aggregated.json"}) ||
		f.Verdict != "" {
		t.Errorf("CatalogueFeedByID(country-de) = %+v, %v", f, ok)
	}
	if got := FeedDisplayName("country-de"); got != "DE" {
		t.Errorf("FeedDisplayName(country-de) = %q, want DE", got)
	}
	if v4, v6 := FeedMaxBits("country-us"); v4 != FeedMaxBitsV4Country || v6 != FeedMaxBitsV6Country {
		t.Errorf("FeedMaxBits(country-us) = /%d, /%d", v4, v6)
	}
	if v4, v6 := FeedMaxBits("dshield"); v4 != FeedMaxBitsV4 || v6 != FeedMaxBitsV6 {
		t.Errorf("FeedMaxBits(dshield) = /%d, /%d", v4, v6)
	}
}

// Spec D5: thirty countries pass, a thirty-first is refused, and the
// catalogue and own feeds beside them do not count.
func TestValidateRules_Countries(t *testing.T) {
	var feeds []string
	for _, cc := range CountryCodes[:MaxCountries] {
		feeds = append(feeds, CountryFeedID(cc))
	}
	withLists := append([]string{"spamhaus-drop", "dshield", "own-1"}, feeds...)
	if err := ValidateRules(Rules{Feeds: withLists}); err != nil {
		t.Errorf("%d countries and three other feeds were refused: %v", MaxCountries, err)
	}
	over := append(slices.Clone(withLists), CountryFeedID(CountryCodes[MaxCountries]))
	if err := ValidateRules(Rules{Feeds: over}); err == nil || !strings.Contains(err.Error(), "at most 30") {
		t.Errorf("a thirty-first country: %v, want a refusal naming the cap", err)
	}
	if err := ValidateRules(Rules{Feeds: []string{"country-zz"}}); err == nil {
		t.Error("country-zz was accepted")
	}
}

// Spec D8: the lists keep their order, the countries follow in theirs.
func TestFeedRuleOrder(t *testing.T) {
	got := FeedRuleOrder([]string{"country-de", "dshield", "country-cn", "own-2", "spamhaus-drop"})
	if want := []string{"dshield", "own-2", "spamhaus-drop", "country-de", "country-cn"}; !slices.Equal(got, want) {
		t.Errorf("FeedRuleOrder = %v, want %v", got, want)
	}
	if got := FeedRuleOrder(nil); len(got) != 0 {
		t.Errorf("FeedRuleOrder(nil) = %v", got)
	}
}

// The cap never goes down: a lower one makes ValidateRules refuse a Current
// rule set that already has more countries on, and that blocks boot, apply and
// save. Raise it if a later release must; never lower it.
func TestMaxCountriesNeverGoesDown(t *testing.T) {
	if MaxCountries < 30 {
		t.Errorf("MaxCountries = %d; 2.26.0 shipped 30 and a rule set may hold that many", MaxCountries)
	}
}

// 2.27.1: every country has its flag in web/static/flags, and nothing else is
// there but flag-icons' licence. Read from git ls-files, so a flag left
// untracked fails here as it would fail a release, and a file for a code no
// longer in CountryCodes — or any stray — fails as stale. npm run build:flags
// writes the directory whole.
func TestEveryCountryHasItsFlag(t *testing.T) {
	root := repoRootDir(t)
	want := map[string]bool{"web/static/flags/LICENSE-flag-icons.txt": true}
	for _, cc := range CountryCodes {
		want["web/static/flags/"+cc+".svg"] = true
	}
	for _, f := range trackedFiles(t, root) {
		if !strings.HasPrefix(f, "web/static/flags/") {
			continue
		}
		if !want[f] {
			t.Errorf("%s is not a flag of a country in CountryCodes, nor the licence: stale (npm run build:flags)", f)
			continue
		}
		if !strings.HasSuffix(f, ".txt") && !strings.HasPrefix(readRepoPath(t, root, f), "<svg ") {
			t.Errorf("%s is not an SVG", f)
		}
		delete(want, f)
	}
	for f := range want {
		t.Errorf("%s is missing (npm run build:flags, then git add)", f)
	}
}
