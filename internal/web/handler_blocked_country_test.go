package web

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
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

// onlyDE is the cell's link to the view of Germany alone (2.29 D11).
const onlyDE = `<a class="link" href="/blocked?country=de">Only this country</a>`

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
		"en": {`<th title="The country the address block is registered to.`, deCell, deCode, `>Germany (DE) ` + onlyDE + `</span>`,
			`aria-label="Block country: Germany">Block country</button>`},
		"de": {`title="Deutschland">DE</button>`, `>Deutschland (DE) <a class="link" href="/blocked?country=de">Nur dieses Land</a></span>`,
			`aria-label="Land sperren: Deutschland">Land sperren</button>`, `data-label="Land"`},
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
			`<span class="cc-pop" id="cc-51" popover>Germany ` + onlyDE + `</span></span>` +
			`<span class="cc-long">Germany (DE) ` + onlyDE + `</span></span>`
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

// ringOf is a ring of n entries, newest first, numbered 1..n: the even ones
// from 203.0.113.200, which the sample table places in RU, the odd ones from
// 203.0.113.9, which it places nowhere.
func ringOf(n uint64) []shared.PacketLogEntry {
	out := make([]shared.PacketLogEntry, 0, n)
	for seq := n; seq >= 1; seq-- {
		e := samplePacket()
		e.Seq = seq
		if seq%2 == 0 {
			e.Src = netip.MustParseAddr("203.0.113.200")
		}
		out = append(out, e)
	}
	return out
}

// ringAsked is what a ringCore was asked, in order.
type ringAsked struct {
	mu   sync.Mutex
	asks []shared.PacketLogFilter
}

func (a *ringAsked) all() []shared.PacketLogFilter {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.asks)
}

// ringCore answers GET_PACKET_LOG from ring the way the core does — the
// filter's Matcher, then EffectiveLimit, Matched counted over the whole ring.
// An old core (before 2.29) does not know before and answers as if it were
// not there.
func ringCore(t *testing.T, ring []shared.PacketLogEntry, old bool) (*fakeCore, *Server, *ringAsked) {
	t.Helper()
	fc, s := blockedCore(t, shared.PacketLogResult{}, shared.FirewallOptions{SSHBruteForceLog: true})
	s.lookup = lookupWith(t, sampleCountryTable(t))
	asked := &ringAsked{}
	fc.Respond(shared.CmdGetPacketLog, func(cmd shared.Command) shared.Response {
		var f shared.PacketLogFilter
		if err := json.Unmarshal(cmd.Payload, &f); err != nil {
			return shared.Response{Error: err.Error()}
		}
		asked.mu.Lock()
		asked.asks = append(asked.asks, f)
		asked.mu.Unlock()
		if old {
			f.Before = 0
		}
		match, err := f.Matcher()
		if err != nil {
			return shared.Response{Error: err.Error()}
		}
		res := shared.PacketLogResult{Listening: true, Held: len(ring), Entries: []shared.PacketLogEntry{}}
		for _, e := range ring {
			if !match(e) {
				continue
			}
			res.Matched++
			if len(res.Entries) < f.EffectiveLimit() {
				res.Entries = append(res.Entries, e)
			}
		}
		return successResp(res)
	})
	return fc, s, asked
}

var seqAttr = regexp.MustCompile(`<tr data-seq="(\d+)"`)

// seqsOf is every row's Seq, in the order drawn.
func seqsOf(t *testing.T, body string) []uint64 {
	t.Helper()
	var out []uint64
	for _, m := range seqAttr.FindAllStringSubmatch(body, -1) {
		n, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

// 2.29 D7: the country filter reads the whole ring, a page of a thousand at a
// time, not the newest 200 — and the count is over the whole ring too.
func TestBlocked_TheCountryFilterPagesThroughTheWholeLog(t *testing.T) {
	_, s, asked := ringCore(t, ringOf(2500), false)
	rec := doAuthRequest(t, s, "GET", "/blocked?country=ru", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, Location %q", rec.Code, rec.Header().Get("Location"))
	}
	body := rec.Body.String()
	seqs := seqsOf(t, body)
	if len(seqs) != 200 {
		t.Fatalf("%d rows, want 200", len(seqs))
	}
	for _, seq := range seqs {
		if seq%2 != 0 {
			t.Fatalf("row %d is not in RU", seq)
		}
	}
	if !strings.Contains(body, `<span class="toolbar-count">200 of 1250</span>`) {
		t.Error("the count is not 200 of 1250, over the whole ring")
	}
	asks := asked.all()
	if len(asks) < 3 {
		t.Fatalf("%d requests: the 2500 entries were not paged through", len(asks))
	}
	for i, f := range asks {
		if f.Limit != shared.PacketLogMaxLimit {
			t.Errorf("request %d asked for %d entries, not %d", i, f.Limit, shared.PacketLogMaxLimit)
		}
		if i > 0 && f.Before == 0 {
			t.Errorf("request %d carried no before", i)
		}
	}
	if strings.Contains(body, "cannot page its log") {
		t.Error("a current core is reported as old")
	}
}

// 2.29 D9: a core older than this interface ignores before and answers the
// newest thousand again. The second answer gives it away: the paging stops
// there, rather than looping, and the page says the view is only the newest.
func TestBlocked_TheCountryFilterStopsAtAnOldCore(t *testing.T) {
	_, s, asked := ringCore(t, ringOf(2500), true)
	body := doAuthRequest(t, s, "GET", "/blocked?country=ru", nil).Body.String()
	if n := len(asked.all()); n != 2 {
		t.Errorf("%d requests to an old core, want 2", n)
	}
	if !strings.Contains(body, "The core is older than this interface and cannot page its log; the country view shows only the newest entries.") {
		t.Error("the page does not say the core is too old to page")
	}
	if n := len(seqsOf(t, body)); n != 200 {
		t.Errorf("%d rows, want 200 of the newest thousand", n)
	}
}

// A code no country has marks its field and filters nothing, like every other
// field that cannot be read.
func TestBlocked_AnUnknownCountryIsMarkedAndFiltersNothing(t *testing.T) {
	_, s, _ := ringCore(t, ringOf(20), false)
	rec := doAuthRequest(t, s, "GET", "/blocked?country=xx", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !regexp.MustCompile(`<select name="country" class="input input-sm is-error" aria-invalid="true">`).MatchString(body) {
		t.Error("the country field is not marked")
	}
	if !slices.Contains(seqsOf(t, body), 19) {
		t.Error("a row outside RU is missing: a filter was applied")
	}
}

// 2.29 D10: while the lookup is off, there is no Country field, and a link
// carrying one lands on the view without it — never an empty one.
func TestBlocked_NoCountryFilterWhileTheLookupIsOff(t *testing.T) {
	for name, lookup := range map[string]func(*testing.T) *countryLookup{
		"off":        func(*testing.T) *countryLookup { return nil },
		"not loaded": func(t *testing.T) *countryLookup { return lookupWith(t, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			_, s, _ := ringCore(t, ringOf(20), false)
			s.lookup = lookup(t)
			rec := doAuthRequest(t, s, "GET", "/blocked?country=ru&port=22", nil)
			assertRedirect(t, rec, "/blocked?port=22")
			body := doAuthRequest(t, s, "GET", "/blocked?port=22", nil).Body.String()
			if strings.Contains(body, `name="country"`) {
				t.Error("a Country field is offered with no table")
			}
			if !slices.Contains(seqsOf(t, body), 19) {
				t.Error("a row outside RU is missing")
			}
			rows := doAuthRequest(t, s, "GET", "/blocked/rows?country=ru", nil).Body.String()
			if !slices.Contains(seqsOf(t, rows), 19) {
				t.Error("the tail filtered by a country it cannot know")
			}
		})
	}
}

// 2.29 D8: the tail with a country costs one request per poll — the newest
// thousand, filtered here — and answers with the country's new rows only, or
// with nothing.
func TestBlocked_TheCountryTailAddsOnlyTheCountrysNewRows(t *testing.T) {
	_, s, asked := ringCore(t, ringOf(2510), false)
	rec := doAuthRequest(t, s, "GET", "/blocked/rows?country=ru&after=2500", nil)
	if got := rec.Header().Get("HX-Reswap"); got != "afterbegin" {
		t.Fatalf("HX-Reswap = %q, want afterbegin", got)
	}
	if got, want := seqsOf(t, rec.Body.String()), []uint64{2510, 2508, 2506, 2504, 2502}; !slices.Equal(got, want) {
		t.Errorf("rows %v, want %v", got, want)
	}
	if n := len(asked.all()); n != 1 {
		t.Errorf("%d requests for one poll, want 1", n)
	}

	// 2511 is new, and not in RU: nothing to add.
	_, s, _ = ringCore(t, ringOf(2511), false)
	if rec := doAuthRequest(t, s, "GET", "/blocked/rows?country=ru&after=2510", nil); rec.Code != http.StatusNoContent {
		t.Errorf("status %d with nothing new in RU, want 204", rec.Code)
	}

	// Pre-flight M6: no after — the page had no rows — is one request too.
	_, s, asked = ringCore(t, ringOf(2510), false)
	rec = doAuthRequest(t, s, "GET", "/blocked/rows?country=ru", nil)
	if got := rec.Header().Get("HX-Reswap"); got != "innerHTML" {
		t.Errorf("HX-Reswap = %q, want innerHTML", got)
	}
	if seqs := seqsOf(t, rec.Body.String()); len(seqs) != 200 || seqs[0] != 2510 || seqs[199]%2 != 0 {
		t.Errorf("%d rows from %v, want RU's newest 200", len(seqs), seqs[:min(len(seqs), 3)])
	}
	if n := len(asked.all()); n != 1 {
		t.Errorf("%d requests with no after, want 1", n)
	}
}

// A ring that went backwards — a core restarted without its spill file — is
// the whole country view again, read the whole way.
func TestBlocked_TheCountryTailRedrawsARingThatWentBackwards(t *testing.T) {
	_, s, _ := ringCore(t, ringOf(40), false)
	rec := doAuthRequest(t, s, "GET", "/blocked/rows?country=ru&after=2500", nil)
	if got := rec.Header().Get("HX-Reswap"); got != "innerHTML" {
		t.Fatalf("HX-Reswap = %q, want innerHTML", got)
	}
	seqs := seqsOf(t, rec.Body.String())
	if len(seqs) != 20 || seqs[0] != 40 || seqs[19] != 2 {
		t.Errorf("rows %v, want RU's twenty, 40 down to 2", seqs)
	}
}

// The country's name leads to the view of only that country, keeping the
// filters already set: in the popover, and on the card a phone shows.
func TestBlocked_TheCountryNameLeadsToOnlyThatCountry(t *testing.T) {
	_, s, _ := ringCore(t, ringOf(2), false)
	body := doAuthRequest(t, s, "GET", "/blocked?port=22", nil).Body.String()
	link := `<a class="link" href="/blocked?country=ru&amp;port=22">Only this country</a>`
	if n := strings.Count(body, link); n != 2 {
		t.Errorf("the RU row carries %d links to only RU, want 2 (popover and card):\n%s", n, body)
	}
	if !strings.Contains(body, `<span class="cc-pop" id="cc-2" popover>Russia `+link+`</span>`) {
		t.Error("the popover does not carry the link")
	}
}
