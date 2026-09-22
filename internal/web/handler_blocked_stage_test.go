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
// The first half stays green even without any unmapping in the handler:
// shared.InAnyEntry unmaps the *entries*, so the stored "::ffff:192.0.2.1"
// still matches the operator in the verdict and the peer check. The second
// half — the zone — is what mutation 4 turns red. The handler's own unmap
// before the "already there" check is pinned by
// TestStageUnmapsBeforeAskingWhetherItIsAlreadyThere, because InAnyEntry does
// not unmap the address it is asked about.
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

// shared.InAnyEntry unmaps the entries, not the address it is asked about, so
// without the handler's own unmap a mapped spelling of a listed address would
// be staged a second time.
func TestStageUnmapsBeforeAskingWhetherItIsAlreadyThere(t *testing.T) {
	s, got, _ := stageCore(t, shared.Rules{TCP: open19999.TCP, Blacklist: []string{"203.0.113.0/24"}})
	rec := postStage(t, s, url.Values{"act": {"blacklist"}, "addr": {"::ffff:203.0.113.9"}}, "192.0.2.1", "")
	if got.called {
		t.Errorf("an address already inside a listed network was staged again: %+v", got.p)
	}
	if f := flashOf(t, s, rec); f != "blocked_refused_already" {
		t.Errorf("flash = %q, want blocked_refused_already", f)
	}
}

// A trusted proxy that sends no X-Forwarded-For: resolveClient falls back to the
// peer and says proxied, so client == peer and only proxied tells the operator
// this address is the proxy, not them.
func TestStageRefusesToBlacklistAProxyThatSendsNoHeader(t *testing.T) {
	s, got, _ := stageCore(t, open19999)
	s.cfg.TrustedProxies = []string{"192.0.2.1"}
	rec := postStage(t, s, url.Values{"act": {"blacklist"}, "addr": {"192.0.2.1"}}, "192.0.2.1", "")
	if got.called {
		t.Fatal("the proxy was staged on the blacklist")
	}
	if f := flashOf(t, s, rec); f != "blocked_refused_proxy" {
		t.Errorf("flash = %q, want blocked_refused_proxy", f)
	}
}

// An untrusted peer that sends a forwarding header about itself is not a proxy
// anything establishes: the header's value is never read, so blacklisting that
// peer is blacklisting the operator, and the sentence has to say so.
func TestStageNamesAnUntrustedPeerWithAHeaderAsTheOperator(t *testing.T) {
	s, got, _ := stageCore(t, open19999)
	rec := postStage(t, s, url.Values{"act": {"blacklist"}, "addr": {"192.0.2.1"}}, "192.0.2.1", "198.51.100.7")
	if got.called {
		t.Fatal("the operator's own address was staged on the blacklist")
	}
	if f := flashOf(t, s, rec); f != "blocked_refused_lockout" {
		t.Errorf("flash = %q, want blocked_refused_lockout", f)
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
	// One case per action that can reach the guard: blacklist is pinned by the
	// lockout tests, and whitelist and open-port only admit more, so failing
	// closed here is the only thing that proves they ask it at all.
	for _, form := range []url.Values{
		{"act": {"whitelist"}, "addr": {"203.0.113.9"}},
		{"act": {"open"}, "proto": {"udp"}, "port": {"51820"}},
	} {
		s, got, fc := stageCore(t, open19999)
		fc.SetResponse(shared.CmdGetSettings, errorRespFor("down"))
		postStage(t, s, form, "192.0.2.1", "")
		if got.called {
			t.Errorf("%v was staged without a verdict", form)
		}
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
