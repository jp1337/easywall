package web

import (
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// lookupWith is a country lookup holding tab, or none when tab is nil. It
// never fetches: its runner is not started.
func lookupWith(t *testing.T, tab *countryTable) *countryLookup {
	t.Helper()
	dir := t.TempDir()
	l := newCountryLookup(filepath.Join(dir, "countries.tar.gz"), newFeedStore(filepath.Join(dir, "feed_fetch.json")))
	if tab != nil {
		l.table.Store(tab)
	}
	return l
}

// Two sources: one the sample table names (DE), one it does not.
func countryPackets() []shared.PacketLogEntry {
	de, none := samplePacket(), samplePacket()
	de.Seq, de.Src = 51, netip.MustParseAddr("192.0.2.80")
	none.Seq = 50 // 203.0.113.9: in no range
	return []shared.PacketLogEntry{de, none}
}

const blockDEForm = `<input type="hidden" name="act" value="country"><input type="hidden" name="cc" value="de">`

// deCode is the cell's code for Germany: a button that opens the name (2.27.1),
// its title for the mouse.
const deCode = `title="Germany">DE</button>`

// deCell opens the Country cell of a source in Germany: the flag first.
const deCell = `data-label="Country"><span class="cc"><img class="flag" src="/static/flags/de.svg?v=`

// Spec D3, amended: off, there is no column and no Block button, and one line
// says which key switches it on. Off is country_lookup = false, and then
// NewServer builds no lookup (TestTheCountryLookupExistsOnlyWhenSwitchedOn).
func TestBlocked_NoCountryColumnWhileTheLookupIsOff(t *testing.T) {
	_, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: countryPackets()},
		shared.FirewallOptions{SSHBruteForceLog: true})
	s.lookup = nil
	body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String()
	for _, absent := range []string{`<th title="The country`, `data-label="Country"`, `value="country"`} {
		if strings.Contains(body, absent) {
			t.Errorf("the lookup is off and the page carries %s", absent)
		}
	}
	if !strings.Contains(body, "<code>country_lookup = true</code> in web.toml adds one.") ||
		!strings.Contains(body, `href="https://easywall-project.org/docs/features/blocked-traffic/#where-it-comes-from"`) {
		t.Error("no line says how to switch the country column on")
	}
}

// On by default: a web.toml that does not name the key has the column, and the
// "how to switch it on" line is not there.
func TestBlocked_TheCountryColumnIsOnByDefault(t *testing.T) {
	_, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: countryPackets()},
		shared.FirewallOptions{SSHBruteForceLog: true})
	if s.lookup == nil {
		t.Fatal("a web.toml without country_lookup built no lookup")
	}
	body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String()
	if !strings.Contains(body, `<th title="The country the address block is registered to.`) {
		t.Error("no Country column with the lookup on by default")
	}
	if strings.Contains(body, "country_lookup = true") {
		t.Error("the 'switch it on' line shows while the lookup is on")
	}
}

// Spec D5, D6: the code, the name as its title, "—" for a source no range
// holds, and a Block button for a country not staged — on the page and in
// the live tail's rows.
func TestBlocked_NamesTheCountryOfEachSource(t *testing.T) {
	fc, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: countryPackets()},
		shared.FirewallOptions{SSHBruteForceLog: true})
	s.lookup = lookupWith(t, sampleCountryTable(t))
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{Staged: shared.Rules{Feeds: []string{"dshield"}}}))

	for lang, want := range map[string][]string{
		"en": {`<th title="The country the address block is registered to.`, deCell, deCode, `>Germany (DE)</span>`,
			`aria-label="Block Germany">Block country</button>`},
		"de": {`title="Deutschland">DE</button>`, `>Deutschland (DE)</span>`,
			`aria-label="Deutschland sperren">Land sperren</button>`, `data-label="Land"`},
	} {
		body := doRequest(s, "GET", "/blocked", nil, makeAuthCookie(t, s), &http.Cookie{Name: LangCookie, Value: lang}).Body.String()
		for _, w := range append(want, blockDEForm) {
			if !strings.Contains(body, w) {
				t.Errorf("%s: the page does not contain %s", lang, w)
			}
		}
		// 203.0.113.9 is in no range: its cell is a dash, and it has no button.
		if n := strings.Count(body, `name="act" value="country"`); n != 1 {
			t.Errorf("%s: %d Block buttons, want 1", lang, n)
		}
	}
	rows := doAuthRequest(t, s, "GET", "/blocked/rows", nil).Body.String()
	if !strings.Contains(rows, deCode) || !strings.Contains(rows, blockDEForm) {
		t.Errorf("the live tail's rows carry no country:\n%s", rows)
	}
}

// 2.27.1: the cell is the flag, the code — a button whose popover holds the
// name, so a tap shows it on a desktop layout — and the full name, which the
// card layout shows instead. The popover is the row's own: its id carries the
// row's Seq, so two rows of one country never open each other's. No country
// is still a dash, with no flag.
func TestBlocked_TheCountryCellIsAFlagACodeAndTheName(t *testing.T) {
	_, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: countryPackets()},
		shared.FirewallOptions{SSHBruteForceLog: true})
	s.lookup = lookupWith(t, sampleCountryTable(t))
	for _, path := range []string{"/blocked", "/blocked/rows"} {
		body := doAuthRequest(t, s, "GET", path, nil).Body.String()
		want := `<span class="cc"><img class="flag" src="/static/flags/de.svg?v=` + shared.CurrentVersion + `" alt="" width="20" height="15">` +
			`<span class="cc-short"><button type="button" class="cc-code" popovertarget="cc-51" title="Germany">DE</button>` +
			`<span class="cc-pop" id="cc-51" popover>Germany</span></span>` +
			`<span class="cc-long">Germany (DE)</span></span>`
		if !strings.Contains(body, want) {
			t.Errorf("%s: no country cell\n%s\nin\n%s", path, want, body)
		}
		if n := strings.Count(body, `class="flag"`); n != 1 {
			t.Errorf("%s: %d flags, want one: 203.0.113.9 is in no country", path, n)
		}
		if !strings.Contains(body, `data-label="Country"><span class="text-ink-subtle">—</span>`) {
			t.Errorf("%s: the source in no country is not a dash", path)
		}
	}
}

// 2.25.2's live tail puts only the new rows on top (HX-Reswap: afterbegin),
// through the same blocked_rows: those rows carry the country cell too, and
// nothing outside the rows — the status line is the page's, not the tail's.
func TestBlocked_TheLiveTailsNewRowsCarryTheCountry(t *testing.T) {
	_, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: countryPackets()},
		shared.FirewallOptions{SSHBruteForceLog: true})
	s.lookup = lookupWith(t, sampleCountryTable(t))
	rec := doAuthRequest(t, s, "GET", "/blocked/rows?after=50", nil)
	if got := rec.Header().Get("HX-Reswap"); got != "afterbegin" {
		t.Fatalf("HX-Reswap = %q, want afterbegin", got)
	}
	body := rec.Body.String()
	if n := strings.Count(body, "<tr data-seq="); n != 1 {
		t.Fatalf("%d rows in the delta, want the one newer than 50", n)
	}
	for _, w := range []string{deCell, deCode, blockDEForm} {
		if !strings.Contains(body, w) {
			t.Errorf("the delta row does not contain %s:\n%s", w, body)
		}
	}
	if strings.Contains(body, "editor-foot") {
		t.Error("the delta carries the status line; it belongs under the table, once")
	}
}

// Spec D6: no Block button for a country already staged, nor for a forwarded
// packet — the countries filter the input chain only — nor when the staged
// rules cannot be read. The cell stays.
func TestBlocked_OffersBlockOnlyWhereItWouldDoSomething(t *testing.T) {
	for name, tc := range map[string]struct {
		rules shared.Response
		hook  string
	}{
		"staged":     {successResp(shared.RulesState{Staged: shared.Rules{Feeds: []string{"country-de"}}}), "input"},
		"forwarded":  {successResp(shared.RulesState{}), "forward"},
		"unreadable": {shared.Response{Success: false, Error: "core busy"}, "input"},
	} {
		t.Run(name, func(t *testing.T) {
			e := countryPackets()[0]
			e.Hook = tc.hook
			fc, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{e}},
				shared.FirewallOptions{SSHBruteForceLog: true})
			s.lookup = lookupWith(t, sampleCountryTable(t))
			fc.SetResponse(shared.CmdGetRules, tc.rules)
			body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String()
			if strings.Contains(body, blockDEForm) {
				t.Error("Block DE is offered")
			}
			if !strings.Contains(body, deCode) {
				t.Error("the country cell is gone")
			}
		})
	}
}

// §1: one line with what the table holds, or that it is not loaded yet, and
// the last failure with the table kept.
func TestBlocked_SaysWhatTheCountryTableHolds(t *testing.T) {
	_, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: countryPackets()},
		shared.FirewallOptions{SSHBruteForceLog: true})

	s.lookup = lookupWith(t, nil)
	if body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String(); !strings.Contains(body, "has not loaded yet") ||
		!strings.Contains(body, `data-label="Country"><span class="text-ink-subtle">—</span>`) {
		t.Error("an empty table: no 'not loaded yet' line, or a row without its dash")
	}

	s.lookup = lookupWith(t, sampleCountryTable(t))
	s.lookup.store.setFetchState(countryLookupKey, feedFetchState{LastAttempt: time.Now(), LastError: feedErrHTTPStatus, HTTPStatus: 503, Failures: 1})
	body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String()
	for _, want := range []string{"ranges in 3 countries", "The last check failed: the server answered 503. The list in use stays."} {
		if !strings.Contains(body, want) {
			t.Errorf("the line does not say %q", want)
		}
	}
}

// Pre-flight M5: the lookup follows GitHub's own redirects and refuses only
// one away from them. The line says that, through ReasonKey, and not the
// feeds' "the server redirected", which would misstate it.
func TestBlocked_ARefusedRedirectIsNamedAsTheLookupsOwn(t *testing.T) {
	_, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: countryPackets()},
		shared.FirewallOptions{SSHBruteForceLog: true})
	s.lookup = lookupWith(t, sampleCountryTable(t))
	s.lookup.store.setFetchState(countryLookupKey, feedFetchState{LastAttempt: time.Now(), LastError: feedErrRedirected, Failures: 1})
	body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String()
	if !strings.Contains(body, "The last check failed: the download was redirected to a host other than GitHub&#39;s, which is refused.") {
		t.Error("the line does not name the lookup's own redirect refusal")
	}
	if strings.Contains(body, "the server redirected") {
		t.Error("the line names the feeds' redirect refusal")
	}
}

// Spec D6: Block stages country-<cc> exactly as the Countries card's Add does
// — every other feed kept, the same cap and refusal — and never applies.
func TestStageACountryFromALogRow(t *testing.T) {
	s, got, fc := stageCore(t, shared.Rules{TCP: open19999.TCP, Feeds: []string{"dshield", "country-aq"}})
	// The first fetch starts at once, as the card's does. The core already
	// holds a copy here, so it asks GET_FEEDS and fetches nothing.
	s.feeds = newFeedRunner(s.client, s.feedStore)
	fc.SetResponse(shared.CmdGetFeeds, successResp(shared.GetFeedsResult{Feeds: []shared.FeedStatus{{ID: "country-de", Stored: true}}}))
	var asked atomic.Int32
	fc.OnCommand(shared.CmdGetFeeds, func(shared.Command) { asked.Add(1) })
	rec := postStage(t, s, url.Values{"act": {"country"}, "cc": {"de"}, "q": {"port=22"}}, "192.0.2.1", "")
	waitUntil(t, 2*time.Second, func() bool { return asked.Load() >= 1 })
	assertRedirect(t, rec, "/blocked?port=22")
	if f := flashOf(t, s, rec); f != "blocked_staged_country" {
		t.Errorf("flash = %q, want blocked_staged_country", f)
	}
	if !got.called || got.p.RuleType != "feeds" {
		t.Fatalf("nothing staged: %+v", got.p)
	}
	var feeds []string
	for _, v := range got.p.Rules.([]interface{}) {
		feeds = append(feeds, v.(string))
	}
	if !slices.Equal(feeds, []string{"dshield", "country-aq", "country-de"}) {
		t.Errorf("staged %v, want dshield and Antarctica kept and country-de added", feeds)
	}
}

func TestStageACountryFromALogRowRefuses(t *testing.T) {
	thirty := []string{}
	for _, cc := range shared.CountryCodes[:shared.MaxCountries] {
		thirty = append(thirty, shared.CountryFeedID(cc))
	}
	for name, tc := range map[string]struct {
		feeds []string
		cc    string
		flash string
	}{
		"already staged":   {[]string{"country-de"}, "de", "blocked_refused_already"},
		"a thirty-first":   {thirty, "zw", "feeds_error_too_many_countries"},
		"no such country":  {nil, "zz", "blocked_refused_invalid"},
		"capitals, forged": {nil, "DE", "blocked_refused_invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			s, got, _ := stageCore(t, shared.Rules{TCP: open19999.TCP, Feeds: tc.feeds})
			rec := postStage(t, s, url.Values{"act": {"country"}, "cc": {tc.cc}}, "192.0.2.1", "")
			if got.called {
				t.Errorf("staged %+v", got.p)
			}
			if f := flashOf(t, s, rec); f != tc.flash {
				t.Errorf("flash = %q, want %s", f, tc.flash)
			}
		})
	}
}
