package web

import (
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// feedsCore is a fake core holding state, answering GET_FEEDS with feeds and
// recording the one SAVE_RULES a feeds save sends.
func feedsCore(t *testing.T, state shared.RulesState, feeds []shared.FeedStatus) (*Server, *fakeCore, *staged) {
	t.Helper()
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetRules, successResp(state))
	fc.SetResponse(shared.CmdGetFeeds, successResp(shared.GetFeedsResult{Feeds: feeds}))
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})
	got := &staged{}
	fc.OnCommand(shared.CmdSaveRules, func(c shared.Command) {
		got.called = true
		_ = json.Unmarshal(c.Payload, &got.p)
	})
	return s, fc, got
}

func stagedIDs(t *testing.T, got *staged) []string {
	t.Helper()
	if !got.called || got.p.RuleType != "feeds" {
		t.Fatalf("no SAVE_RULES(feeds) was sent: %+v", got)
	}
	raw, _ := json.Marshal(got.p.Rules)
	var ids []string
	if err := json.Unmarshal(raw, &ids); err != nil {
		t.Fatal(err)
	}
	return ids
}

// Rules.Feeds is "in the order they were switched on" (interfaces.md): the
// ids that stay keep their place, the new ones follow in the card's order.
func TestStagingFeedsKeepsTheOrderTheyWereSwitchedOn(t *testing.T) {
	none := func(int) bool { return false }
	next, refused := stagedFeeds(
		[]string{"dshield", "spamhaus-drop", "cins"},
		[]string{"spamhaus-drop", "et-compromised", "dshield", "blocklist-de", "dshield"},
		nil, none)
	if want := []string{"dshield", "spamhaus-drop", "blocklist-de", "et-compromised"}; refused != "" || !slices.Equal(next, want) {
		t.Errorf("stagedFeeds = %v, %q; want %v", next, refused, want)
	}
	if next, refused := stagedFeeds([]string{"dshield"}, nil, nil, none); refused != "" || next == nil || len(next) != 0 {
		t.Errorf("every switch off = %#v, %q; want an empty list, not null", next, refused)
	}
	if _, refused := stagedFeeds(nil, []string{"firehol-level1"}, nil, none); refused != "save_error" {
		t.Errorf("an id nothing knows: %q", refused)
	}
}

// Spec §1: a ✗ feed asks for explicit confirmation before it is staged. The
// server holds it, so a form posted without JavaScript is held to it too.
func TestADeliberateFeedNeedsItsOwnConfirmation(t *testing.T) {
	for name, c := range map[string]struct {
		before []string
		form   url.Values
		want   []string // nil: refused
	}{
		"no box":            {nil, url.Values{"feed": {"tor-exits"}}, nil},
		"another feed's":    {nil, url.Values{"feed": {"tor-exits", "ipsum-3"}, "confirm": {"ipsum-3"}}, nil},
		"ticked":            {nil, url.Values{"feed": {"tor-exits"}, "confirm": {"tor-exits"}}, []string{"tor-exits"}},
		"already staged":    {[]string{"hagezi-tif"}, url.Values{"feed": {"hagezi-tif"}}, []string{"hagezi-tif"}},
		"a ✓ needs no box":  {nil, url.Values{"feed": {"spamhaus-drop"}}, []string{"spamhaus-drop"}},
		"a • needs no box":  {nil, url.Values{"feed": {"cins"}}, []string{"cins"}},
		"only in the query": {nil, url.Values{"feed": {"tor-exits"}}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			s, _, got := feedsCore(t, shared.RulesState{Staged: shared.Rules{Feeds: c.before}}, nil)
			path := "/blocklist/feeds"
			if name == "only in the query" {
				path += "?confirm=tor-exits"
			}
			rec := doAuthFormRequest(t, s, path, c.form.Encode())
			assertRedirect(t, rec, "/blocklist#feeds")
			if c.want == nil {
				if got.called {
					t.Fatalf("staged without its confirmation: %+v", got.p)
				}
				if f := flashOf(t, s, rec); f != "feeds_refused_confirm" {
					t.Errorf("flash = %q, want feeds_refused_confirm", f)
				}
				return
			}
			if ids := stagedIDs(t, got); !slices.Equal(ids, c.want) {
				t.Errorf("staged %v, want %v", ids, c.want)
			}
			if f := flashOf(t, s, rec); f != "saved" {
				t.Errorf("flash = %q, want saved", f)
			}
		})
	}
}

// An own feed with no address has nothing to fetch; one already staged (an
// import, P14) may still be switched off, and stay on.
func TestAnOwnFeedIsSwitchedOnOnlyOnceItHasAnAddress(t *testing.T) {
	s, _, got := feedsCore(t, shared.RulesState{Staged: shared.Rules{Feeds: []string{"own-2"}}}, nil)
	rec := doAuthFormRequest(t, s, "/blocklist/feeds", "feed=own-1&feed=own-2")
	if got.called || flashOf(t, s, rec) != "feeds_refused_unconfigured" {
		t.Fatalf("an own feed with no address was staged: %+v, flash %q", got.p, flashOf(t, s, rec))
	}
	if err := s.saveOwnFeed(1, OwnFeed{Name: "Mine", URL: "https://example.org/list.txt"}); err != nil {
		t.Fatal(err)
	}
	doAuthFormRequest(t, s, "/blocklist/feeds", "feed=own-1&feed=own-2")
	if ids := stagedIDs(t, got); !slices.Equal(ids, []string{"own-2", "own-1"}) {
		t.Errorf("staged %v", ids)
	}
}

// The first fetch starts at once for a feed just switched on (spec §3), and
// only for those: the runner asks the core for its copy (GET_FEEDS) before it
// fetches, so every GET_FEEDS here is one refreshNow. The core answers that
// it holds every copy, so nothing reaches the network.
func TestSavingTheFeedsFetchesOnlyTheNewOnes(t *testing.T) {
	var all []shared.FeedStatus
	for _, f := range shared.FeedCatalogue {
		all = append(all, shared.FeedStatus{ID: f.ID, Stored: true})
	}
	s, fc, got := feedsCore(t, shared.RulesState{Staged: shared.Rules{Feeds: []string{"dshield"}}}, all)
	s.feeds = newFeedRunner(s.client, s.feedStore)
	var asked atomic.Int32
	fc.OnCommand(shared.CmdGetFeeds, func(shared.Command) { asked.Add(1) })

	doAuthFormRequest(t, s, "/blocklist/feeds", "feed=dshield&feed=cins&feed=spamhaus-drop")
	if ids := stagedIDs(t, got); !slices.Equal(ids, []string{"dshield", "spamhaus-drop", "cins"}) {
		t.Fatalf("staged %v", ids)
	}
	waitUntil(t, 2*time.Second, func() bool { return asked.Load() >= 2 })
	time.Sleep(150 * time.Millisecond)
	if n := asked.Load(); n != 2 {
		t.Errorf("%d first fetches for two newly switched-on feeds", n)
	}
}

// Everything the card shows about a row, in the three positions a switch has
// relative to the kernel (D2), and the four warnings (spec §5).
func TestTheFeedsCardSaysWhatIsStagedAndWhatIsLive(t *testing.T) {
	old := time.Now().Add(-40 * 24 * time.Hour)
	s, _, _ := feedsCore(t, shared.RulesState{
		Staged:  shared.Rules{Feeds: []string{"spamhaus-drop", "cins", "own-1"}},
		Current: shared.Rules{Feeds: []string{"spamhaus-drop", "dshield", "own-1"}},
	}, []shared.FeedStatus{
		{ID: "spamhaus-drop", Stored: true, Entries: 1801, ChangedAt: old, CheckedAt: time.Now(),
			InKernel: true, Packets: 4242, CountersRead: true, AllowlistOverlap: 3},
		{ID: "dshield", Stored: true, Entries: 20, ChangedAt: time.Now(), InKernel: true, Packets: 99, CountersRead: false},
		{ID: "own-1", InKernel: true},
	})
	s.feedStore.setFetchState("cins", feedFetchState{Failures: 4, LastError: feedErrHTTPStatus, HTTPStatus: 503, Status: FeedFailedNoCopy})
	s.feedStore.setFetchState("dshield", feedFetchState{Failures: 1, LastError: feedErrShrank, Status: FeedFailedCopy,
		ShrankFrom: 20, ShrankTo: 3})
	if err := s.saveOwnFeed(1, OwnFeed{Name: "CrowdSec", URL: "https://admin.example.net:8443/v1/integrations/SECRETID/content",
		User: "machine", Password: "s3cr3t-p4ss"}); err != nil {
		t.Fatal(err)
	}
	body := doAuthRequest(t, s, "GET", "/blocklist", nil).Body.String()
	row := func(id string) string {
		t.Helper()
		m := regexp.MustCompile(`(?s)<div class="module" id="feed-` + regexp.QuoteMeta(id) + `">.*?\n</div>`).FindString(body)
		if m == "" {
			t.Fatalf("no row for %s:\n%s", id, body)
		}
		return m
	}

	sp, ds, ci, own := row("spamhaus-drop"), row("dshield"), row("cins"), row("own-1")
	for what, c := range map[string]struct {
		in, text string
	}{
		"on and live":        {sp, "Active"},
		"staged, not live":   {ci, "On once you apply"},
		"live, not staged":   {ds, "Off once you apply — active until then"},
		"its entries":        {sp, ">1801<"},
		"its packets":        {sp, ">4242<"},
		"stale":              {sp, "Unchanged for 30 days"},
		"allowlist overlap":  {sp, "Contains allowlist entries (3), which stay reachable."},
		"failures":           {ci, "Failed 4 times in a row."},
		"the status":         {ci, "Failed — no copy yet"},
		"the reason":         {ci, "the server answered 503"},
		"an empty set":       {own, "In the kernel with an empty set"},
		"a refused shrink":   {ds, "Refused: shrank from 20 to 3. To accept the smaller list, switch the feed off and apply, then switch it on and apply."},
		"an own feed's host": {own, ">admin.example.net:8443<"},
		"the verdict":        {sp, "✓ Recommended"},
		"false positives":    {sp, "Practically none."},
	} {
		if !strings.Contains(c.in, c.text) {
			t.Errorf("%s: %q missing from\n%s", what, c.text, c.in)
		}
	}
	if strings.Contains(ds, ">99<") {
		t.Error("packets shown for a feed whose counters were not read")
	}
	if strings.Contains(ci, "Blocked since") {
		t.Error("packets shown for a feed that is not in the kernel")
	}
	if !strings.Contains(sp, `value="spamhaus-drop" class="toggle" checked`) || strings.Contains(ds, `class="toggle" checked`) {
		t.Error("the switch is not the staged position")
	}
	if off := row("blocklist-de"); strings.Contains(off, "Last refresh") || strings.Contains(off, "State") {
		t.Errorf("a feed that is off shows refresh facts:\n%s", off)
	}

	// P17: every catalogue row links its source and its terms, in a new tab
	// that cannot reach back.
	links := regexp.MustCompile(`<a class="link" href="(https://[^"]+)" target="_blank" rel="noopener noreferrer">`).FindAllStringSubmatch(body, -1)
	if len(links) != 2*len(shared.FeedCatalogue) {
		t.Errorf("%d source/terms links, want %d", len(links), 2*len(shared.FeedCatalogue))
	}
	for _, f := range shared.FeedCatalogue {
		if !strings.Contains(row(f.ID), `href="`+f.Homepage+`"`) || !strings.Contains(row(f.ID), `href="`+f.Terms+`"`) {
			t.Errorf("%s does not link its homepage and terms", f.ID)
		}
	}

	// An own feed's path carries the integration id and the password is
	// write-only: neither is anywhere in the page but the edit field's URL.
	if strings.Contains(body, "s3cr3t-p4ss") {
		t.Error("the stored password is in the page")
	}
	if strings.Count(body, "SECRETID") != 1 {
		t.Errorf("the own feed's path appears %d times; only its own edit field may carry it", strings.Count(body, "SECRETID"))
	}
	if pw := regexp.MustCompile(`<input type="password"[^>]*>`).FindAllString(body, -1); len(pw) != shared.MaxOwnFeeds {
		t.Errorf("%d password fields, want %d", len(pw), shared.MaxOwnFeeds)
	} else {
		for _, f := range pw {
			if strings.Contains(f, "value=") {
				t.Errorf("a password field is pre-filled: %s", f)
			}
		}
		if !strings.Contains(pw[0], `placeholder="Stored — leave empty to keep it"`) || strings.Contains(pw[1], "placeholder") {
			t.Errorf("the placeholder does not say which slot has a password stored: %v", pw)
		}
	}
	if !strings.Contains(body, `id="own-1-user" name="user" maxlength="256" class="input w-full" autocomplete="off"`) {
		t.Error("the user field invites the browser's saved login")
	}
}

// A ✗ row carries its confirmation box, naming whom it refuses, until it is
// staged; no ✓ or • row does.
func TestOnlyADeliberateFeedAsksForConfirmation(t *testing.T) {
	s, _, _ := feedsCore(t, shared.RulesState{Staged: shared.Rules{Feeds: []string{"hagezi-tif"}}}, nil)
	body := doAuthRequest(t, s, "GET", "/blocklist", nil).Body.String()
	boxes := regexp.MustCompile(`name="confirm" value="([a-z0-9-]+)"`).FindAllStringSubmatch(body, -1)
	var got []string
	for _, b := range boxes {
		got = append(got, b[1])
	}
	if want := []string{"ipsum-3", "tor-exits"}; !slices.Equal(got, want) {
		t.Errorf("confirmation boxes on %v, want %v (hagezi-tif is staged already)", got, want)
	}
	if !strings.Contains(body, "I know this refuses every Tor user, legitimate or not.") {
		t.Error("the Tor box does not say whom it refuses")
	}
}

// The core not answering GET_FEEDS is not zero entries: the card says it
// could not read them and shows none. The rules not answering leaves no card:
// its switches would all read "off", and one save would switch every feed off.
func TestTheFeedsCardSaysWhatItCouldNotRead(t *testing.T) {
	s, fc, _ := feedsCore(t, shared.RulesState{Staged: shared.Rules{Feeds: []string{"dshield"}}, Current: shared.Rules{Feeds: []string{"dshield"}}}, nil)
	fc.SetResponse(shared.CmdGetFeeds, errorRespFor("timeout"))
	body := doAuthRequest(t, s, "GET", "/blocklist", nil).Body.String()
	if !strings.Contains(body, "The core did not answer, so entries, times and packet counts are not shown.") {
		t.Error("an unreadable GET_FEEDS is not said")
	}
	if strings.Contains(body, "<dt>Entries</dt>") {
		t.Error("entries shown although the core did not answer")
	}
	fc.SetResponse(shared.CmdGetRules, errorRespFor("down"))
	if body := doAuthRequest(t, s, "GET", "/blocklist", nil).Body.String(); strings.Contains(body, `action="/blocklist/feeds"`) {
		t.Error("the feeds form was drawn without knowing what is staged")
	}
}

// Every refusal saveOwnFeed can give has its message, beside the slot, with
// what was typed — except the password.
func TestAnOwnFeedRefusalShowsBesideItsSlot(t *testing.T) {
	s, _, _ := feedsCore(t, shared.RulesState{}, nil)
	form := url.Values{"n": {"2"}, "name": {"Mine"}, "url": {"http://example.org/list.txt"},
		"user": {"me"}, "password": {"hunter2-typed"}}
	rec := doAuthFormRequest(t, s, "/blocklist/own-feeds", form.Encode())
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `<p class="field-error" role="alert">Not saved: the address must be https://`) {
		t.Fatalf("answered %d without the reason:\n%s", rec.Code, body)
	}
	if !strings.Contains(body, `value="http://example.org/list.txt"`) || !strings.Contains(body, `id="own-2-name" name="name" maxlength="64" class="input w-full" autocomplete="off"
                 value="Mine"`) {
		t.Error("what was typed was thrown away")
	}
	if strings.Contains(body, "hunter2-typed") {
		t.Error("the typed password was written back into the page")
	}
	if _, ok := s.feedStore.ownFeed(2); ok {
		t.Error("a refused own feed was stored")
	}
	for err, key := range map[error]string{
		errOwnFeedName: "feeds_own_err_name", errOwnFeedURL: "feeds_own_err_url",
		errOwnFeedLogin: "feeds_own_err_login", errOwnFeedDemo: "feeds_own_err_demo", errOwnFeedSlot: "save_error",
		errOwnFeedPasswordAgain: "feeds_own_err_password_again",
	} {
		if got := ownFeedErrKey(err); got != key {
			t.Errorf("%v → %q, want %q", err, got, key)
		}
	}

	// Refused for carrying credentials: shown back without them (R3#7).
	form.Set("url", "https://u:url-secret@example.org/list.txt")
	body = doAuthFormRequest(t, s, "/blocklist/own-feeds", form.Encode()).Body.String()
	if strings.Contains(body, "url-secret") || !strings.Contains(body, `value="https://example.org/list.txt"`) {
		t.Error("a URL refused for its credentials was written back with them")
	}

	form.Set("url", "https://example.org/list.txt")
	assertRedirect(t, doAuthFormRequest(t, s, "/blocklist/own-feeds", form.Encode()), "/blocklist#own-feeds")
	if f, ok := s.feedStore.ownFeed(2); !ok || f.Password != "hunter2-typed" {
		t.Fatalf("not stored: %+v", f)
	}
	doAuthFormRequest(t, s, "/blocklist/own-feeds", "n=2&name=Mine&url=https%3A%2F%2Fexample.org%2Flist.txt&clear=1")
	if _, ok := s.feedStore.ownFeed(2); ok {
		t.Error("Remove did not clear the slot")
	}
}

// Carry-in (controller ruling, Task 6 → Task 7): errOwnFeedPasswordAgain
// (feed_state.go) fires both when a stored feed's host changes under an empty
// password field and on a first save that gives a user but no password —
// there being nothing to send either way. The save handler renders it beside
// its slot like every other errOwnFeed*, and the password field stays empty:
// the one field the page never writes back, refused or not.
func TestAnOwnFeedNeedsItsPasswordRetypedForANewAddress(t *testing.T) {
	s, _, _ := feedsCore(t, shared.RulesState{}, nil)

	// A first save: a user with no password has nothing to send.
	form := url.Values{"n": {"3"}, "name": {"Mine"}, "url": {"https://example.org/list.txt"}, "user": {"me"}}
	body := doAuthFormRequest(t, s, "/blocklist/own-feeds", form.Encode()).Body.String()
	if !strings.Contains(body, `<p class="field-error" role="alert">Not saved: this address needs its own password`) {
		t.Fatalf("a first save with a user and no password does not say so:\n%s", body)
	}
	if pw := regexp.MustCompile(`<input type="password"[^>]*>`).FindAllString(body, -1); len(pw) != shared.MaxOwnFeeds {
		t.Fatalf("%d password fields, want %d", len(pw), shared.MaxOwnFeeds)
	} else {
		for _, f := range pw {
			if strings.Contains(f, "value=") {
				t.Errorf("a password field carries a value: %s", f)
			}
		}
	}
	if _, ok := s.feedStore.ownFeed(3); ok {
		t.Error("a refused first save was stored")
	}

	// Store it properly, then point it at a new host with the password field
	// left empty: the old password must not follow it there.
	form.Set("password", "first-pass")
	assertRedirect(t, doAuthFormRequest(t, s, "/blocklist/own-feeds", form.Encode()), "/blocklist#own-feeds")
	form2 := url.Values{"n": {"3"}, "name": {"Mine"}, "url": {"https://elsewhere.example.org/list.txt"}, "user": {"me"}}
	body = doAuthFormRequest(t, s, "/blocklist/own-feeds", form2.Encode()).Body.String()
	if !strings.Contains(body, `<p class="field-error" role="alert">Not saved: this address needs its own password`) {
		t.Fatalf("a changed host with no retyped password does not say so:\n%s", body)
	}
	if strings.Contains(body, "first-pass") {
		t.Error("the stored password was written back into the page")
	}
	if f, ok := s.feedStore.ownFeed(3); !ok || f.URL != "https://example.org/list.txt" || f.Password != "first-pass" {
		t.Errorf("a refused save overwrote the stored address: %+v", f)
	}
}

// The demo refuses own feeds (a password typed into a public page would be
// kept on its host) and says so before anyone types.
func TestTheDemoRefusesOwnFeedsAndSaysSo(t *testing.T) {
	s := newDemoTestServer(t)
	rec := doAuthFormRequest(t, s, "/blocklist/own-feeds", "n=1&name=x&url=https%3A%2F%2Fexample.org%2Fl.txt")
	assertRedirect(t, rec, "/blocklist#own-feeds")
	if f := flashOf(t, s, rec); f != "feeds_own_err_demo" {
		t.Errorf("flash = %q", f)
	}
	body := doAuthRequest(t, s, "GET", "/blocklist", nil).Body.String()
	if !strings.Contains(body, "The public demo keeps no own feeds") ||
		len(regexp.MustCompile(`<input type="password"[^>]*\sdisabled>`).FindAllString(body, -1)) != shared.MaxOwnFeeds {
		t.Error("the demo's own-feed form does not say it keeps nothing")
	}
}

// The demo's card shows every status a row has but one, and every warning
// (newDemoFeedStore's table); the fifth status is a visitor's own switch.
func TestTheDemoCardShowsEveryState(t *testing.T) {
	s := newDemoTestServer(t)
	body := doAuthRequest(t, s, "GET", "/blocklist", nil).Body.String()
	for _, want := range []string{
		"Updated", "Unchanged", "Failed — the previous copy stays active", "Failed — no copy yet",
		"Unchanged for 30 days", "Contains allowlist entries (1)", "Failed 3 times in a row.",
		"In the kernel with an empty set", "Refused: shrank from 32002 to 9140.", "the server answered 503",
		"The demo fetches no lists.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the demo's card never shows %q", want)
		}
	}
	doAuthFormRequest(t, s, "/blocklist/feeds", "feed=spamhaus-drop&feed=dshield&feed=blocklist-de&feed=et-compromised&feed=cins&feed=ipsum-3&confirm=ipsum-3")
	body = doAuthRequest(t, s, "GET", "/blocklist", nil).Body.String()
	if !strings.Contains(body, "Never fetched") || !strings.Contains(body, "On once you apply") {
		t.Error("a feed a visitor switches on in the demo does not read never fetched, on once applied")
	}
}

// The label guard for the computed keys the card asks for — T (printf
// "feed_status_%s"), feed_error_%s, a ✗ feed's ConfirmKey — which
// TestTemplatesOnlyUseTranslatedKeys cannot see.
func TestEveryFeedStateHasItsWords(t *testing.T) {
	for _, lang := range StrictLangs {
		strs := localeStrings(t, lang)
		need := func(id string) {
			t.Helper()
			if strings.TrimSpace(strs[id]) == "" {
				t.Errorf("locales/%s.json has no %q", lang, id)
			}
		}
		for _, k := range AllFeedStatusKinds {
			need("feed_status_" + string(k))
		}
		for _, r := range AllFeedErrors {
			need("feed_error_" + r)
		}
		for _, f := range shared.FeedCatalogue {
			if f.Verdict == shared.FeedDeliberate {
				need(shared.FeedLocaleKey(f.ID, "confirm"))
			}
		}
	}
}

// Spec §5: a feed row on /blocked is "Feed: <name>" — the catalogue's, an own
// feed's configured name, "Own feed N" for an unconfigured one — and leads to
// that feed's row on /blocklist. The remedy beside it is the allowlist.
func TestAFeedRowNamesItsFeedAndLeadsToIt(t *testing.T) {
	for _, c := range []struct {
		feed, chip string
	}{
		{"spamhaus-drop", `<a class="log-action" href="/blocklist#feed-spamhaus-drop">Feed: Spamhaus DROP</a>`},
		{"own-1", `<a class="log-action" href="/blocklist#feed-own-1">Feed: CrowdSec</a>`},
		{"own-2", `<a class="log-action" href="/blocklist#feed-own-2">Feed: Own feed 2</a>`},
		{"", `<a class="log-action" href="/blocklist#feeds">Feed</a>`},
	} {
		e := samplePacket()
		e.Rule, e.Feed = "feed", c.feed
		fc, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{e}},
			shared.FirewallOptions{LogFeed: true})
		fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{}))
		if err := s.saveOwnFeed(1, OwnFeed{Name: "CrowdSec", URL: "https://example.org/l.txt"}); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/blocked/rows", "/blocked"} {
			body := doAuthRequest(t, s, "GET", path, nil).Body.String()
			if !strings.Contains(body, c.chip) {
				t.Errorf("%s, feed %q: no %s in\n%s", path, c.feed, c.chip, body)
			}
			if !strings.Contains(body, `name="act" value="allowlist"`) {
				t.Errorf("%s, feed %q: no allowlist remedy", path, c.feed)
			}
		}
	}
}

// The card's refusals are amber — something to tick or type, nothing failed —
// and a stored own feed is the action working (DESIGN.md § Status).
func TestTheFeedFlashesCarryTheirTone(t *testing.T) {
	flashClass := templateFuncs()["flashClass"].(func(string) string)
	for key, want := range map[string]string{
		"feeds_refused_confirm": "alert-warn", "feeds_refused_unconfigured": "alert-warn",
		"feeds_own_err_name": "alert-warn", "feeds_own_err_url": "alert-warn",
		"feeds_own_err_login": "alert-warn", "feeds_own_err_demo": "alert-warn",
		"feeds_own_saved": "alert-ok",
	} {
		if got := flashClass(key); got != want {
			t.Errorf("flashClass(%q) = %q, want %q", key, got, want)
		}
	}
}

// A refused blocklist save re-renders the page with the typed text; the Feeds
// card below it stays, read from what is staged, not dropped from the page.
func TestARefusedBlocklistSaveKeepsTheFeedsCard(t *testing.T) {
	s, _, _ := feedsCore(t, shared.RulesState{Staged: shared.Rules{Feeds: []string{"dshield"}}}, nil)
	body := doAuthFormRequest(t, s, "/blocklist", "entries=not-an-address").Body.String()
	if !strings.Contains(body, `action="/blocklist/feeds"`) || !strings.Contains(body, `value="dshield" class="toggle" checked`) {
		t.Error("the Feeds card is missing after a refused blocklist save")
	}
}

// Spec §5: the order-matters aside is the three steps, and only them — the
// paragraph that repeated them is gone (ruling X8), and its concrete trap, an
// allowlist entry inside a blocked range, is step 1's.
func TestTheOrderAsideIsTheThreeSteps(t *testing.T) {
	s, _, _ := feedsCore(t, shared.RulesState{}, nil)
	body := doAuthRequest(t, s, "GET", "/blocklist", nil).Body.String()
	aside := regexp.MustCompile(`(?s)Order matters\s*</h2>(.*?)</div>\s*</div>\s*</div>`).FindStringSubmatch(body)
	if aside == nil {
		t.Fatalf("no order-matters aside:\n%s", body)
	}
	if n := strings.Count(aside[1], `<li class="step">`); n != 3 {
		t.Errorf("%d steps, want 3", n)
	}
	if !regexp.MustCompile(`</ol>\s*$`).MatchString(aside[1]) {
		t.Errorf("something follows the steps:\n%s", aside[1])
	}
	if !strings.Contains(aside[1], "not even an allowlist entry inside a blocked range") {
		t.Error("step 1 lost the concrete case")
	}
}
