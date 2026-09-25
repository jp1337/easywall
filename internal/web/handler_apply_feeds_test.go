package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func verdictFrom(t *testing.T, s *Server, staged shared.Rules) *applyVerdict {
	t.Helper()
	req := httptest.NewRequest("GET", "/apply", nil)
	req.RemoteAddr = "203.0.113.7:40000"
	return s.reachVerdict(req, staged, shared.FirewallOptions{}, shared.NetworkSettings{})
}

// Spec §4: the lockout verdict reads "your address is in feed X". The hits
// come from GET_FEEDS asked about this request's address, and the feed is
// named by the first staged feed that holds it.
func TestReachVerdict_NamesTheFeedThatDropsTheOperator(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetFeeds, successResp(shared.GetFeedsResult{Feeds: []shared.FeedStatus{
		{ID: "cins", Stored: true}, {ID: "dshield", Stored: true, ContainsAddr: true},
		{ID: "spamhaus-drop", Stored: true, ContainsAddr: true},
	}}))
	staged := shared.Rules{TCP: open19999.TCP, Feeds: []string{"cins", "spamhaus-drop", "dshield"}}

	v := verdictFrom(t, s, staged)
	if v.Verdict != shared.ReachBlocked || v.Reason != shared.ReasonInFeed || v.Feed != "Spamhaus DROP" {
		t.Errorf("verdict %+v, want blocked by Spamhaus DROP", v)
	}
	var asked shared.GetFeedsPayload
	if cmd := fc.LastCommand(); cmd == nil || cmd.Type != shared.CmdGetFeeds ||
		json.Unmarshal(cmd.Payload, &asked) != nil || asked.Addr != "203.0.113.7" {
		t.Errorf("the core was asked %+v, want GET_FEEDS about 203.0.113.7", cmd)
	}

	// Fix round 1, Finding 1: a second staged feed with no answer yet must not
	// swallow the name of the one that already decided it. Before the fix this
	// fell into the "unknown" branch — which recomputes and finds the verdict
	// unchanged (still blocked by spamhaus-drop) — and the naming, guarded by
	// a switch case exclusive with "unknown", never ran: "in the feed , which
	// drops it" instead of "in the feed Spamhaus DROP".
	fc2 := newFakeCore(t)
	s2 := newTestServer(t, fc2)
	fc2.SetResponse(shared.CmdGetFeeds, successResp(shared.GetFeedsResult{Feeds: []shared.FeedStatus{
		{ID: "cins", Stored: false}, {ID: "spamhaus-drop", Stored: true, ContainsAddr: true},
		{ID: "dshield", Stored: true},
	}}))
	staged2 := shared.Rules{TCP: open19999.TCP, Feeds: []string{"cins", "spamhaus-drop", "dshield"}}
	v2 := verdictFrom(t, s2, staged2)
	if v2.Verdict != shared.ReachBlocked || v2.Reason != shared.ReasonInFeed || v2.Feed != "Spamhaus DROP" {
		t.Errorf("verdict %+v, want blocked by Spamhaus DROP even with cins unanswered", v2)
	}
}

// A GET_FEEDS that fails is not "in no feed". Where a hit would change the
// answer the verdict is unknown; where it would not — the allowlist is first —
// the verdict stands. With no feed staged the core is not asked at all.
func TestReachVerdict_AnUnreadableFeedAnswerIsUnknownOnlyWhereItMatters(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetFeeds, errorRespFor("i/o timeout"))

	v := verdictFrom(t, s, shared.Rules{TCP: open19999.TCP, Feeds: []string{"dshield"}})
	if v.Verdict != shared.ReachUnknown || v.Reason != shared.ReasonFeedsUnreadable {
		t.Errorf("verdict %+v, want unknown/feeds_unreadable", v)
	}
	v = verdictFrom(t, s, shared.Rules{Allowlist: []string{"203.0.113.7"}, Feeds: []string{"dshield"}})
	if v.Verdict != shared.ReachOpen || v.Reason != shared.ReasonAllowlisted {
		t.Errorf("verdict %+v, want open/allowlisted — the allowlist does not depend on the feeds", v)
	}

	fc2 := newFakeCore(t)
	s2 := newTestServer(t, fc2)
	fc2.SetResponse(shared.CmdGetFeeds, errorRespFor("must not be asked"))
	if v := verdictFrom(t, s2, open19999); v.Verdict != shared.ReachOpen {
		t.Errorf("with no feed staged the verdict is %+v", v)
	}
	if cmd := fc2.LastCommand(); cmd != nil && cmd.Type == shared.CmdGetFeeds {
		t.Error("GET_FEEDS was asked with no feed staged")
	}
}

// And the page says it: the template hands the feed's name to reach_in_feed.
func TestHandleApplyGET_TheVerdictNamesTheFeed(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{HasPending: true}))
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Staged: shared.Rules{TCP: open19999.TCP, Feeds: []string{"dshield"}},
	}))
	fc.SetResponse(shared.CmdGetOptions, successResp(shared.FirewallOptions{}))
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))
	fc.SetResponse(shared.CmdGetAppliedConfig, successResp(shared.AppliedConfigResult{}))
	fc.SetResponse(shared.CmdGetFeeds, successResp(shared.GetFeedsResult{Feeds: []shared.FeedStatus{
		{ID: "dshield", Stored: true, ContainsAddr: true},
	}}))

	rec := doAuthRequest(t, s, "GET", "/apply", nil)
	assertStatus(t, rec, http.StatusOK)
	if body := rec.Body.String(); !strings.Contains(body, "in the feed DShield top block list") {
		t.Errorf("the verdict does not name the feed:\n%s", body)
	}
}

// Carry-in (controller ruling, Task 6 → Task 7): shared.FeedDisplayName
// always renders the English literal "Own feed N" — it knows no locale and no
// configured name. The lockout sentence (here) and the apply diff must use
// the operator's own name once one is set, else a label in the page's own
// language, never that literal on a German page.
func TestReachVerdict_AnOwnFeedIsNamedInThePageLanguage(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetFeeds, successResp(shared.GetFeedsResult{Feeds: []shared.FeedStatus{
		{ID: "own-2", Stored: true, ContainsAddr: true},
	}}))
	staged := shared.Rules{TCP: open19999.TCP, Feeds: []string{"own-2"}}

	req := httptest.NewRequest("GET", "/apply", nil)
	req.RemoteAddr = "203.0.113.7:40000"
	req.AddCookie(&http.Cookie{Name: LangCookie, Value: "de"})
	if v := s.reachVerdict(req, staged, shared.FirewallOptions{}, shared.NetworkSettings{}); v.Feed != "Eigene Fremdliste 2" {
		t.Errorf("an unconfigured own feed's German lockout name = %q, want the localized default, not the English literal", v.Feed)
	}

	// Configured, the operator's own name is used regardless of the page's
	// language — it is theirs, not translated.
	if err := s.saveOwnFeed(2, OwnFeed{Name: "CrowdSec", URL: "https://example.org/l.txt"}); err != nil {
		t.Fatal(err)
	}
	if v := s.reachVerdict(req, staged, shared.FirewallOptions{}, shared.NetworkSettings{}); v.Feed != "CrowdSec" {
		t.Errorf("a configured own feed's lockout name = %q, want its own name", v.Feed)
	}

	// The apply diff renders the same label, staging own-2 from nothing.
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetStatus, successResp(shared.FirewallStatus{HasPending: true}))
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{Staged: staged}))
	fc.SetResponse(shared.CmdGetOptions, successResp(shared.FirewallOptions{}))
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))
	fc.SetResponse(shared.CmdGetAppliedConfig, successResp(shared.AppliedConfigResult{}))
	authCookie := makeAuthCookie(t, s)
	rec := doRequest(s, "GET", "/apply", nil, authCookie, &http.Cookie{Name: LangCookie, Value: "de"})
	assertStatus(t, rec, http.StatusOK)
	if body := rec.Body.String(); strings.Contains(body, "Own feed 2") {
		t.Errorf("the apply diff shows the English literal on a German page:\n%s", body)
	} else if !strings.Contains(body, "CrowdSec") {
		t.Errorf("the apply diff does not name the own feed by its configured name:\n%s", body)
	}
}

// Rulings X4: a staged feed with no stored copy yet — reported with Stored
// false, or not listed at all — makes the verdict unknown where a hit would
// change it: its copy arrives later and loads with no acceptance window.
func TestReachVerdict_AFeedWithNoCopyYetIsUnknown(t *testing.T) {
	for name, feeds := range map[string][]shared.FeedStatus{
		"reported without a copy": {{ID: "dshield", Stored: false}},
		"not listed":              {},
	} {
		t.Run(name, func(t *testing.T) {
			fc := newFakeCore(t)
			s := newTestServer(t, fc)
			fc.SetResponse(shared.CmdGetFeeds, successResp(shared.GetFeedsResult{Feeds: feeds}))
			v := verdictFrom(t, s, shared.Rules{TCP: open19999.TCP, Feeds: []string{"dshield"}})
			if v.Verdict != shared.ReachUnknown || v.Reason != shared.ReasonFeedsUnreadable {
				t.Errorf("verdict %+v, want unknown/feeds_unreadable", v)
			}
		})
	}
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetFeeds, successResp(shared.GetFeedsResult{Feeds: []shared.FeedStatus{{ID: "dshield", Stored: true}}}))
	if v := verdictFrom(t, s, shared.Rules{TCP: open19999.TCP, Feeds: []string{"dshield"}}); v.Verdict != shared.ReachOpen {
		t.Errorf("a stored copy without the address: %+v, want open", v)
	}
}

// Fix round 1, Finding 2: staging a /blocked row action asks the lockout
// question twice — before the edit and after — and both ask about the same
// operator address with (for every row action this handler stages) the same
// staged.Feeds. GET_FEEDS is asked once, not twice, for the two verdicts.
func TestStageAsksGetFeedsOnlyOnceForTheLockoutCheck(t *testing.T) {
	rules := shared.Rules{TCP: open19999.TCP, Feeds: []string{"dshield"}}
	s, got, fc := stageCore(t, rules)
	fc.SetResponse(shared.CmdGetFeeds, successResp(shared.GetFeedsResult{Feeds: []shared.FeedStatus{
		{ID: "dshield", Stored: true},
	}}))
	asked := 0
	fc.OnCommand(shared.CmdGetFeeds, func(shared.Command) { asked++ })

	rec := postStage(t, s, url.Values{"act": {"blocklist"}, "addr": {"203.0.113.9"}}, "192.0.2.1", "")
	assertRedirect(t, rec, "/blocked")
	if !got.called {
		t.Fatal("nothing was staged")
	}
	if asked != 1 {
		t.Errorf("GET_FEEDS was asked %d times, want exactly 1", asked)
	}
}
