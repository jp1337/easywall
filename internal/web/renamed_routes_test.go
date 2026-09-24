package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// A bookmark from before 2.23 moves permanently to the page's new address.
func TestTheOldListAddressesMovePermanently(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	for old, now := range map[string]string{"/blacklist": "/blocklist", "/whitelist": "/allowlist"} {
		rec := doAuthRequest(t, s, "GET", old, nil)
		if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != now {
			t.Errorf("GET %s → %d %q, want 301 %q", old, rec.Code, rec.Header().Get("Location"), now)
		}
	}
}

// A tab left open across the upgrade posts to the old address. The list it
// carries is saved to the list it names — a redirect would have turned the
// POST into a GET and thrown the text away.
func TestAPostToAnOldListAddressIsSaved(t *testing.T) {
	for old, want := range map[string]string{"/blacklist": "blocklist", "/whitelist": "allowlist"} {
		fc := newFakeCore(t)
		s := newTestServer(t, fc)
		enrollFactor(t, s)
		var got shared.SaveRulesPayload
		fc.OnCommand(shared.CmdSaveRules, func(c shared.Command) { _ = json.Unmarshal(c.Payload, &got) })

		rec := doAuthFormRequest(t, s, old, "entries=203.0.113.9")
		assertRedirect(t, rec, "/"+want)
		if list, _ := got.Rules.([]interface{}); got.RuleType != want || len(list) != 1 || list[0] != "203.0.113.9" {
			t.Errorf("POST %s saved %+v, want 203.0.113.9 in %s", old, got, want)
		}
	}
}

// A /blocked filter saved before 2.23 names rule=blacklist. It is read as the
// blocklist rule and the URL is moved on, rather than refused as a rule
// easywall does not log.
func TestAnOldBlockedFilterReadsAsTheNewRule(t *testing.T) {
	f, bad := blockedFilter(url.Values{"rule": {"blacklist"}})
	if bad != "" || f.Rule != "blocklist" {
		t.Fatalf("rule=blacklist read as %q (bad %q), want blocklist", f.Rule, bad)
	}

	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	rec := doAuthRequest(t, s, "GET", "/blocked?rule=blacklist", nil)
	assertRedirect(t, rec, "/blocked?rule=blocklist")
}

// The same stale /blocked page's row action posts act=blacklist.
func TestAnOldRowActionStagesOnTheNewList(t *testing.T) {
	s, got, _ := stageCore(t, open19999)
	postStage(t, s, url.Values{"act": {"blacklist"}, "addr": {"203.0.113.9"}}, "192.0.2.1", "")
	if !got.called || got.p.RuleType != "blocklist" {
		t.Fatalf("act=blacklist staged %+v, want the blocklist", got.p)
	}
}
