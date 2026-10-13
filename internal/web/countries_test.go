package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// Spec §2: the Countries card stages the country subset of Rules.Feeds and
// keeps every other id where it was.
func TestTheCountriesCardKeepsEveryOtherFeed(t *testing.T) {
	before := []string{"dshield", "country-cn", "own-1"}
	for name, c := range map[string]struct {
		form, flash string
		want        []string // nil: nothing staged
	}{
		"add":             {"add=country-de", "saved", []string{"dshield", "country-cn", "own-1", "country-de"}},
		"remove":          {"remove=country-cn", "saved", []string{"dshield", "own-1"}},
		"remove wins":     {"add=country-de&remove=country-cn", "saved", []string{"dshield", "own-1"}},
		"already on":      {"add=country-cn", "saved", before},
		"not a country":   {"add=dshield", "save_error", nil},
		"removing a list": {"remove=dshield", "save_error", nil},
		"an unknown code": {"add=country-zz", "save_error", nil},
		"nothing":         {"", "save_error", nil},
	} {
		t.Run(name, func(t *testing.T) {
			s, _, got := feedsCore(t, shared.RulesState{Staged: shared.Rules{Feeds: before}}, nil)
			rec := doAuthFormRequest(t, s, "/blocklist/countries", c.form)
			assertRedirect(t, rec, "/blocklist#countries")
			if f := flashOf(t, s, rec); f != c.flash {
				t.Errorf("flash %q, want %q", f, c.flash)
			}
			if c.want == nil {
				if got.called {
					t.Errorf("staged %+v", got.p)
				}
				return
			}
			if ids := stagedIDs(t, got); !slices.Equal(ids, c.want) {
				t.Errorf("staged %v, want %v", ids, c.want)
			}
		})
	}
}

// Spec D5, in the form: the thirty-first country is refused before the core
// is asked, with a sentence that says how many may be on.
func TestTheThirtyFirstCountryIsRefused(t *testing.T) {
	var thirty []string
	for _, cc := range shared.CountryCodes[:shared.MaxCountries] {
		thirty = append(thirty, shared.CountryFeedID(cc))
	}
	s, _, got := feedsCore(t, shared.RulesState{Staged: shared.Rules{Feeds: append([]string{"dshield"}, thirty...)}}, nil)
	rec := doAuthFormRequest(t, s, "/blocklist/countries", "add="+shared.CountryFeedID(shared.CountryCodes[shared.MaxCountries]))
	if got.called || flashOf(t, s, rec) != "feeds_error_too_many_countries" {
		t.Errorf("a thirty-first country: staged %v, flash %q", got.called, flashOf(t, s, rec))
	}
	if c := templateFuncs()["flashClass"].(func(string) string)("feeds_error_too_many_countries"); c != "alert-warn" {
		t.Errorf("the refusal renders %s; nothing failed, so amber", c)
	}
}

// The Feeds card's form carries no country. Saving it kept only the ids it
// posted, which switched every country off.
func TestSavingTheFeedsCardKeepsTheCountries(t *testing.T) {
	s, _, got := feedsCore(t, shared.RulesState{Staged: shared.Rules{Feeds: []string{"country-de", "dshield"}}}, nil)
	doAuthFormRequest(t, s, "/blocklist/feeds", "feed=dshield&feed=cins")
	if ids := stagedIDs(t, got); !slices.Equal(ids, []string{"country-de", "dshield", "cins"}) {
		t.Errorf("staged %v, want the country kept", ids)
	}
}

// Spec §2: adding a country starts its first fetch, as switching on a feed
// does; removing one does not. Every GET_FEEDS here is one refreshNow.
func TestAddingACountryFetchesItAndRemovingDoesNot(t *testing.T) {
	s, fc, _ := feedsCore(t, shared.RulesState{Staged: shared.Rules{Feeds: []string{"country-cn"}}},
		[]shared.FeedStatus{{ID: "country-de", Stored: true}, {ID: "country-cn", Stored: true}})
	s.feeds = newFeedRunner(s.client, s.feedStore)
	var asked atomic.Int32
	fc.OnCommand(shared.CmdGetFeeds, func(shared.Command) { asked.Add(1) })

	// A browser sends the select's country with every press, the remove
	// button's included.
	doAuthFormRequest(t, s, "/blocklist/countries", "add=country-de&remove=country-cn")
	time.Sleep(150 * time.Millisecond)
	if n := asked.Load(); n != 0 {
		t.Fatalf("removing a country fetched %d times", n)
	}
	doAuthFormRequest(t, s, "/blocklist/countries", "add=country-de")
	waitUntil(t, 2*time.Second, func() bool { return asked.Load() >= 1 })
}

// Spec D7: a country's name is CLDR's, in the page's language.
func TestACountryIsNamedInThePageLanguage(t *testing.T) {
	for _, c := range []struct{ lang, cc, want string }{
		{"de", "de", "Deutschland"}, {"en", "de", "Germany"}, {"fr", "kp", "Corée du Nord"}, {"en", "us", "United States"},
	} {
		if got := countryName(c.lang, c.cc); got != c.want {
			t.Errorf("countryName(%s, %s) = %q, want %q", c.lang, c.cc, got, c.want)
		}
	}
	s := newTestServer(t, newFakeCore(t))
	tFunc := func(id string, _ ...interface{}) string { return id }
	if got := s.feedLabel(tFunc, "de", "country-de"); got != "Deutschland" {
		t.Errorf("feedLabel(de, country-de) = %q, want Deutschland", got)
	}
}

// Spec §2: the /blocked chip says Country and leads to the country's row.
func TestACountryChipNamesTheCountryAndLeadsToIt(t *testing.T) {
	e := samplePacket()
	e.Rule, e.Feed = "feed", "country-de"
	fc, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{e}},
		shared.FirewallOptions{LogFeed: true})
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{}))
	for lang, chip := range map[string]string{
		"en": `<a class="log-action" href="/blocklist#country-de">Country: Germany</a>`,
		"de": `<a class="log-action" href="/blocklist#country-de">Land: Deutschland</a>`,
	} {
		body := doRequest(s, "GET", "/blocked", nil, makeAuthCookie(t, s), &http.Cookie{Name: LangCookie, Value: lang}).Body.String()
		if !strings.Contains(body, chip) {
			t.Errorf("%s: no %s in\n%s", lang, chip, body)
		}
	}
}

// Spec D8, on /apply: an address in a list and in a country is named by the
// list, whose rule comes first — whichever was switched on first. The diff
// names a country in the page's language.
func TestTheVerdictNamesTheListBeforeTheCountry(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetFeeds, successResp(shared.GetFeedsResult{Feeds: []shared.FeedStatus{
		{ID: "country-de", Stored: true, ContainsAddr: true}, {ID: "dshield", Stored: true, ContainsAddr: true},
	}}))
	staged := shared.Rules{TCP: open19999.TCP, Feeds: []string{"country-de", "dshield"}}
	if v := verdictFrom(t, s, staged); v.Reason != shared.ReasonInFeed || v.Feed != "DShield top block list" {
		t.Errorf("verdict %+v, want the list named", v)
	}

	req := httptest.NewRequest("GET", "/apply", nil)
	req.AddCookie(&http.Cookie{Name: LangCookie, Value: "de"})
	deltas := shared.DiffRules(shared.Rules{}, shared.Rules{Feeds: []string{"country-de"}})
	s.feedDiffLabels(req, deltas)
	if len(deltas) != 1 || deltas[0].Label != "Deutschland" {
		t.Errorf("diff %+v, want the country labelled Deutschland", deltas)
	}
}

// The refusal states the cap from shared.MaxCountries, not from a digit in a
// locale file that raising the cap would leave behind.
func TestTheCountryRefusalStatesTheCap(t *testing.T) {
	var thirty []string
	for _, cc := range shared.CountryCodes[:shared.MaxCountries] {
		thirty = append(thirty, shared.CountryFeedID(cc))
	}
	s, _, _ := feedsCore(t, shared.RulesState{Staged: shared.Rules{Feeds: thirty}}, nil)
	rec := doAuthFormRequest(t, s, "/blocklist/countries", "add="+shared.CountryFeedID(shared.CountryCodes[shared.MaxCountries]))
	body := doRequest(s, "GET", "/apply", nil, rec.Result().Cookies()[0], makeAuthCookie(t, s)).Body.String()
	if want := fmt.Sprintf("at most %d countries", shared.MaxCountries); !strings.Contains(body, want) {
		t.Errorf("no %q in\n%s", want, body)
	}

	// The rendering alone passes with a literal 30 in the locale file, so the
	// strings themselves must carry {{.Max}} and no digit. Every key that
	// states the cap belongs in this list.
	capKeys := []string{"feeds_error_too_many_countries"}
	for _, lang := range []string{"en", "de"} {
		raw, err := os.ReadFile(filepath.Join(localesDir(t), lang+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var msgs []struct{ ID, Translation string }
		if err := json.Unmarshal(raw, &msgs); err != nil {
			t.Fatal(err)
		}
		for _, key := range capKeys {
			i := slices.IndexFunc(msgs, func(m struct{ ID, Translation string }) bool { return m.ID == key })
			if i < 0 {
				t.Errorf("%s: no %s", lang, key)
				continue
			}
			if tr := msgs[i].Translation; !strings.Contains(tr, "{{.Max}}") || strings.ContainsAny(tr, "0123456789") {
				t.Errorf("%s %s = %q: want {{.Max}} and no digit", lang, key, tr)
			}
		}
	}
}

// An apply while a switched-on country has no copy loads an empty set, and the
// operator confirms it. The copy then arrives with the next refresh, with no
// acceptance window, and their own address may be in it. So /apply refuses
// and names the country; a list keeps today's behaviour.
func TestApplyWaitsForACountrysFirstCopy(t *testing.T) {
	for name, c := range map[string]struct {
		staged []string
		feeds  []shared.FeedStatus // nil: GET_FEEDS fails
		flash  string              // "": applied
	}{
		"a country with no copy": {[]string{"dshield", "country-de"}, []shared.FeedStatus{{ID: "dshield", Stored: true}}, "apply_country_not_loaded"},
		"a country with a copy":  {[]string{"dshield", "country-de"}, []shared.FeedStatus{{ID: "dshield", Stored: true}, {ID: "country-de", Stored: true}}, ""},
		"a list with no copy":    {[]string{"cins"}, []shared.FeedStatus{}, ""},
		"the core cannot say":    {[]string{"country-de"}, nil, "apply_error"},
	} {
		t.Run(name, func(t *testing.T) {
			s, fc, _ := feedsCore(t, shared.RulesState{Staged: shared.Rules{Feeds: c.staged}}, c.feeds)
			if c.feeds == nil {
				fc.SetResponse(shared.CmdGetFeeds, errorRespFor("no answer"))
			}
			var applied atomic.Bool
			fc.OnCommand(shared.CmdApplyRules, func(shared.Command) { applied.Store(true) })
			rec := doAuthFormRequest(t, s, "/apply/start", "")
			assertRedirect(t, rec, "/apply")
			if f := flashOf(t, s, rec); f != c.flash || applied.Load() != (c.flash == "") {
				t.Errorf("flash %q, applied %v; want flash %q", f, applied.Load(), c.flash)
			}
		})
	}

	s, _, _ := feedsCore(t, shared.RulesState{Staged: shared.Rules{Feeds: []string{"country-de"}}}, nil)
	for lang, want := range map[string]string{"en": "Germany has no copy yet", "de": "für Deutschland liegt noch keine Kopie"} {
		lc := &http.Cookie{Name: LangCookie, Value: lang}
		rec := doFormRequest(s, "POST", "/apply/start", "", makeAuthCookie(t, s), lc)
		body := doRequest(s, "GET", "/apply", nil, rec.Result().Cookies()[0], makeAuthCookie(t, s), lc).Body.String()
		if !strings.Contains(body, want) {
			t.Errorf("%s: no %q in\n%s", lang, want, body)
		}
	}
}
