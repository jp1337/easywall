package web

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// open19999 is the port newTestServer binds, open to everyone, so the operator's
// verdict starts out "open" and a flip to "blocked" is this action's doing.
var open19999 = shared.Rules{TCP: []shared.PortRule{{Port: "19999", Description: "easywall"}}}

type staged struct {
	called bool
	p      shared.SaveRulesPayload
}

func stageCore(t *testing.T, rules shared.Rules) (*Server, *staged, *fakeCore) {
	t.Helper()
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{Current: rules, Staged: rules}))
	fc.SetResponse(shared.CmdGetOptions, successResp(shared.FirewallOptions{}))
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})
	got := &staged{}
	fc.OnCommand(shared.CmdSaveRules, func(c shared.Command) {
		got.called = true
		_ = json.Unmarshal(c.Payload, &got.p)
	})
	return s, got, fc
}

// postStage sends the form from peer, optionally through a forwarding header.
func postStage(t *testing.T, s *Server, form url.Values, peer, xff string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/blocked/stage", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = peer + ":40000"
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	req.AddCookie(makeAuthCookie(t, s))
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

func flashOf(t *testing.T, s *Server, rec *httptest.ResponseRecorder) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	sess, _ := s.store.Get(req, SessionName)
	f, _ := sess.Values["flash"].(string)
	return f
}

// Spec §6, the trap: the operator's own address is in the log because the SSH
// module rate-limited them, and one click would blacklist them out.
func TestStageRefusesToBlacklistTheOperator(t *testing.T) {
	s, got, _ := stageCore(t, open19999)
	rec := postStage(t, s, url.Values{"act": {"blacklist"}, "addr": {"192.0.2.1"}}, "192.0.2.1", "")
	assertRedirect(t, rec, "/blocked")
	if got.called {
		t.Fatalf("the operator's own address was staged on the blacklist: %+v", got.p)
	}
	if f := flashOf(t, s, rec); f != "blocked_refused_lockout" {
		t.Errorf("flash = %q, want blocked_refused_lockout", f)
	}
}

// The verdict half of the guard, isolated: behind a trusted proxy the operator
// is 198.51.100.7 and the peer is the proxy, so blacklisting the operator's own
// address flips the verdict while the peer check sees nothing.
func TestStageRefusesWhenTheVerdictFlips(t *testing.T) {
	s, got, _ := stageCore(t, open19999)
	s.cfg.TrustedProxies = []string{"192.0.2.1"}
	rec := postStage(t, s, url.Values{"act": {"blacklist"}, "addr": {"198.51.100.7"}}, "192.0.2.1", "198.51.100.7")
	if got.called {
		t.Fatal("blacklisting the operator's resolved address was staged")
	}
	if f := flashOf(t, s, rec); f != "blocked_refused_lockout" {
		t.Errorf("flash = %q, want blocked_refused_lockout", f)
	}
}

// The proxy half, isolated: the address in the log is the proxy's. The verdict
// about the operator (198.51.100.7) stays open — the confident answer about the
// wrong host the spec warns of — and only the peer check refuses.
func TestStageRefusesToBlacklistTheProxy(t *testing.T) {
	s, got, _ := stageCore(t, open19999)
	s.cfg.TrustedProxies = []string{"192.0.2.1"}
	rec := postStage(t, s, url.Values{"act": {"blacklist"}, "addr": {"192.0.2.1"}}, "192.0.2.1", "198.51.100.7")
	if got.called {
		t.Fatal("the proxy every request arrives through was staged on the blacklist")
	}
	if f := flashOf(t, s, rec); f != "blocked_refused_proxy" {
		t.Errorf("flash = %q, want blocked_refused_proxy", f)
	}
}

// Somebody else's address, the ordinary case: staged, not applied.
func TestStageBlacklistsAStranger(t *testing.T) {
	s, got, _ := stageCore(t, open19999)
	rec := postStage(t, s, url.Values{"act": {"blacklist"}, "addr": {"203.0.113.9"}, "q": {"port=22"}}, "192.0.2.1", "")
	assertRedirect(t, rec, "/blocked?port=22")
	if !got.called || got.p.RuleType != "blacklist" {
		t.Fatalf("nothing was staged: %+v", got.p)
	}
	list, _ := got.p.Rules.([]interface{})
	if len(list) != 1 || list[0] != "203.0.113.9" {
		t.Errorf("staged %v, want [203.0.113.9]", got.p.Rules)
	}
	if f := flashOf(t, s, rec); f != "blocked_staged_blacklist" {
		t.Errorf("flash = %q", f)
	}
}

// Review Focus 1: one spelling for the guard and the kernel.
//
// The first half proves the storing side only: shared.InAnyEntry and
// shared.Reachable both unmap on their own, so this half stays green even
// without any unmapping in the handler itself. The checking half is what the
// mutation table's mutation 4 actually exercises — recorded here rather than
// dropping the assertion, since it still pins the behaviour even though it
// does not by itself distinguish "the handler unmaps" from "nothing here
// needed to".
func TestStageUnmapsTheAddressBeforeCheckingAndStoring(t *testing.T) {
	s, got, _ := stageCore(t, open19999)
	postStage(t, s, url.Values{"act": {"blacklist"}, "addr": {"::ffff:192.0.2.1"}}, "192.0.2.1", "")
	if got.called {
		t.Error("the operator's address in its IPv4-mapped spelling slipped past the guard")
	}

	s, got, _ = stageCore(t, open19999)
	postStage(t, s, url.Values{"act": {"whitelist"}, "addr": {"fe80::1%eth0"}}, "192.0.2.1", "")
	list, _ := got.p.Rules.([]interface{})
	if len(list) != 1 || list[0] != "fe80::1" {
		t.Errorf("stored %v, want the unzoned fe80::1", got.p.Rules)
	}
}

func TestStageOpensAPort(t *testing.T) {
	s, got, _ := stageCore(t, open19999)
	rec := postStage(t, s, url.Values{"act": {"open"}, "proto": {"udp"}, "port": {"51820"}}, "192.0.2.1", "")
	if !got.called || got.p.RuleType != "udp" {
		t.Fatalf("staged %+v", got.p)
	}
	raw, _ := json.Marshal(got.p.Rules)
	var rules []shared.PortRule
	_ = json.Unmarshal(raw, &rules)
	if len(rules) != 1 || rules[0].Port != "51820" || len(rules[0].Sources) != 0 {
		t.Errorf("staged %+v, want one unrestricted rule for 51820", rules)
	}
	if f := flashOf(t, s, rec); f != "blocked_staged_port" {
		t.Errorf("flash = %q", f)
	}
}

func TestStageRefusesWhatIsAlreadyThere(t *testing.T) {
	rules := shared.Rules{
		TCP:       open19999.TCP,
		Blacklist: []string{"203.0.113.0/24"},
	}
	for _, form := range []url.Values{
		{"act": {"blacklist"}, "addr": {"203.0.113.9"}},        // inside a listed network
		{"act": {"open"}, "proto": {"tcp"}, "port": {"19999"}}, // already open to everyone
	} {
		s, got, _ := stageCore(t, rules)
		rec := postStage(t, s, form, "192.0.2.1", "")
		if got.called {
			t.Errorf("%v was staged a second time", form)
		}
		if f := flashOf(t, s, rec); f != "blocked_refused_already" {
			t.Errorf("%v: flash = %q", form, f)
		}
	}
}

func TestStageRefusesWhatItCannotRead(t *testing.T) {
	for _, form := range []url.Values{
		{"act": {"blacklist"}, "addr": {"203.0.113.0/24"}}, // a network is typed on the blacklist page
		{"act": {"blacklist"}, "addr": {"nope"}},
		{"act": {"open"}, "proto": {"icmp"}, "port": {"1"}},
		{"act": {"open"}, "proto": {"tcp"}, "port": {"0"}},
		{"act": {"apply"}},
	} {
		s, got, _ := stageCore(t, open19999)
		rec := postStage(t, s, form, "192.0.2.1", "")
		if got.called {
			t.Errorf("%v was staged", form)
		}
		if f := flashOf(t, s, rec); f != "blocked_refused_invalid" {
			t.Errorf("%v: flash = %q", form, f)
		}
	}
}

// The question cannot be asked, so nothing is staged. Failing open here would
// make a broken socket the one condition under which the guard does nothing.
func TestStageRefusesWhenTheVerdictCannotBeAsked(t *testing.T) {
	s, got, fc := stageCore(t, open19999)
	fc.SetResponse(shared.CmdGetSettings, errorRespFor("down"))
	postStage(t, s, url.Values{"act": {"whitelist"}, "addr": {"203.0.113.9"}}, "192.0.2.1", "")
	if got.called {
		t.Error("staged without a verdict")
	}
}

// Spec §6: they stage, they do not apply.
func TestStageNeverApplies(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{Current: open19999, Staged: open19999}))
	fc.SetResponse(shared.CmdGetOptions, successResp(shared.FirewallOptions{}))
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))
	fc.SetResponse(shared.CmdSaveRules, shared.Response{Success: true})
	applied := false
	fc.OnCommand(shared.CmdApplyRules, func(shared.Command) { applied = true })
	postStage(t, s, url.Values{"act": {"whitelist"}, "addr": {"203.0.113.9"}}, "192.0.2.1", "")
	if applied {
		t.Error("a row action applied the rules; it must only stage them")
	}
}

func TestStage_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	assertRedirect(t, doFormRequest(s, "POST", "/blocked/stage", "act=whitelist&addr=203.0.113.9"), "/login")
}
