package web

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// Every reason has a sentence in every strict locale. The template asks for it
// through printf, which the template-key guard cannot see.
func TestEveryDropReasonHasALabel(t *testing.T) {
	for lang := range strictLangSet() {
		ids := localeIDs(t, lang)
		for _, c := range shared.AllDropReasons {
			if !ids["blocked_why_"+string(c)] {
				t.Errorf("locales/%s.json has no blocked_why_%s", lang, c)
			}
		}
	}
}

// Each sentence names only what DropReason hands it. A placeholder the code
// does not fill renders as "<no value>" on the page, in one language only.
func TestEveryDropReasonSentenceIsFilled(t *testing.T) {
	rules := shared.Rules{
		TCP:       []shared.PortRule{{Port: "22"}, {Port: "8443", Sources: []string{"10.0.0.0/8"}}, {Port: "9000", Scope: shared.ScopeForwarded}},
		Blocklist: []string{"192.0.2.66"}, Allowlist: []string{"198.51.100.7"},
	}
	filter := shared.NetworkSettings{IPv6: shared.IPv6Config{Mode: shared.IPv6Filter}}
	tcp := func(src string, port uint16) shared.PacketLogEntry {
		a := netip.MustParseAddr(src)
		fam := uint8(4)
		if a.Is6() {
			fam = 6
		}
		return shared.PacketLogEntry{Rule: "drop", Family: fam, Proto: "tcp", Src: a, DstPort: port}
	}
	icmp := func(typ uint8, set bool) shared.PacketLogEntry {
		e := tcp("203.0.113.9", 0)
		e.Proto = "icmp"
		if set {
			e.ICMP = &shared.PacketICMP{Type: typ}
		}
		return e
	}
	gre := tcp("203.0.113.9", 0)
	gre.Proto = "47"
	whys := []shared.DropWhy{
		tcp("203.0.113.9", 993).DropReason(rules, filter),
		tcp("203.0.113.9", 22).DropReason(rules, filter),
		tcp("203.0.113.9", 8443).DropReason(rules, filter),
		tcp("203.0.113.9", 9000).DropReason(rules, filter),
		tcp("192.0.2.66", 22).DropReason(rules, filter),
		tcp("198.51.100.7", 22).DropReason(rules, filter),
		tcp("2001:db8::9", 22).DropReason(rules, shared.NetworkSettings{IPv6: shared.IPv6Config{Mode: shared.IPv6Block}}),
		tcp("2001:db8::9", 22).DropReason(rules, shared.NetworkSettings{IPv6: shared.IPv6Config{Mode: shared.IPv6Passthrough}}),
		icmp(8, true).DropReason(rules, filter),
		icmp(13, true).DropReason(rules, filter),
		icmp(3, true).DropReason(rules, filter),
		icmp(0, false).DropReason(rules, filter),
		gre.DropReason(rules, filter),
	}
	covered := map[shared.DropReason]bool{}
	for lang := range strictLangSet() {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Language", lang)
		loc := NewLocalizer(testBundle(t), req, lang)
		for _, w := range whys {
			covered[w.Code] = true
			got := T(loc, "blocked_why_"+string(w.Code), w.Params)
			if strings.Contains(got, "<no value>") || strings.HasPrefix(got, "blocked_why_") {
				t.Errorf("%s %s: %q", lang, w.Code, got)
			}
		}
	}
	for _, c := range shared.AllDropReasons {
		if !covered[c] {
			t.Errorf("no case here renders %s — add one", c)
		}
	}
}

func whyCore(t *testing.T, e shared.PacketLogEntry, rules shared.Rules, recorded bool) string {
	t.Helper()
	fc, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{e}},
		shared.FirewallOptions{LogBlocked: true})
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{Current: rules}))
	fc.SetResponse(shared.CmdGetAppliedConfig, successResp(shared.AppliedConfigResult{Recorded: recorded,
		Config: shared.AppliedConfig{Network: shared.NetworkSettings{IPv6: shared.IPv6Config{Mode: shared.IPv6Filter}}}}))
	return doAuthRequest(t, s, "GET", "/blocked/rows", nil).Body.String()
}

// 2.23 G1: a default-drop row says why, from the rules applied now.
func TestBlockedDefaultDropSaysWhy(t *testing.T) {
	drop := samplePacket()
	drop.Rule, drop.DstPort = "drop", 993
	rules := shared.Rules{TCP: []shared.PortRule{{Port: "22"}}}
	if body := whyCore(t, drop, rules, true); !strings.Contains(body, `<span class="pkt-why">Port 993/tcp is not open.</span>`) {
		t.Errorf("a default drop to a closed port does not say so:\n%s", body)
	}
	// The Staged set is not what the kernel holds: a port staged open is still
	// closed until it is applied.
	fc, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{drop}},
		shared.FirewallOptions{LogBlocked: true})
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{Staged: shared.Rules{TCP: []shared.PortRule{{Port: "993"}}}}))
	fc.SetResponse(shared.CmdGetAppliedConfig, successResp(shared.AppliedConfigResult{Recorded: true}))
	if body := doAuthRequest(t, s, "GET", "/blocked/rows", nil).Body.String(); !strings.Contains(body, "Port 993/tcp is not open.") {
		t.Errorf("the reason was read from the staged rules, not the applied ones:\n%s", body)
	}
	// Unknown is not a reason: no applied snapshot, no sentence.
	if body := whyCore(t, drop, rules, false); strings.Contains(body, "pkt-why") {
		t.Errorf("a reason was given without knowing the applied settings:\n%s", body)
	}
	// A module row names itself; it gets no reason.
	ssh := samplePacket()
	if body := whyCore(t, ssh, rules, true); strings.Contains(body, "pkt-why") {
		t.Errorf("an SSH brute-force row was given a default-drop reason:\n%s", body)
	}
	// Review Focus 4: a core that times out on the extra reads. No reason is
	// shown, but the row and the tail still render — GetRules failing must not
	// take the page down with it.
	fc, s = blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{drop}},
		shared.FirewallOptions{LogBlocked: true})
	fc.SetResponse(shared.CmdGetRules, shared.Response{Success: false, Error: "timeout"})
	// Without this, GET_APPLIED_CONFIG sits at fakeCore's default
	// {Success:true}, which decodes as Recorded:false — that alone would hide
	// the reason and pass the assertion below for the wrong half.
	fc.SetResponse(shared.CmdGetAppliedConfig, successResp(shared.AppliedConfigResult{Recorded: true}))
	rec := doAuthRequest(t, s, "GET", "/blocked/rows", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("a core that times out on GET_RULES answered %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "993") {
		t.Errorf("the row itself is missing when GET_RULES times out:\n%s", body)
	} else if strings.Contains(body, "pkt-why") {
		t.Errorf("a reason was given although the rules could not be read:\n%s", body)
	}
	// The mirror: GET_RULES succeeds, GET_APPLIED_CONFIG times out. Either half
	// missing must hide the reason, not just one of them.
	fc, s = blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{drop}},
		shared.FirewallOptions{LogBlocked: true})
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{Current: rules}))
	fc.SetResponse(shared.CmdGetAppliedConfig, shared.Response{Success: false, Error: "timeout"})
	rec = doAuthRequest(t, s, "GET", "/blocked/rows", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("a core that times out on GET_APPLIED_CONFIG answered %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "993") {
		t.Errorf("the row itself is missing when GET_APPLIED_CONFIG times out:\n%s", body)
	} else if strings.Contains(body, "pkt-why") {
		t.Errorf("a reason was given although the applied config could not be read:\n%s", body)
	}
}

// The reason is only asked for when a default-drop row is on screen: two more
// socket round trips per five-second poll are spent only when they buy
// something.
func TestBlockedRowsAskForTheRulesOnlyForADefaultDrop(t *testing.T) {
	fc, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{samplePacket()}},
		shared.FirewallOptions{LogBlocked: true})
	asked := 0
	fc.OnCommand(shared.CmdGetRules, func(shared.Command) { asked++ })
	doAuthRequest(t, s, "GET", "/blocked/rows", nil)
	if asked != 0 {
		t.Errorf("the tail read the rules %d times for a page with no default-drop row", asked)
	}
}

// 2.23 G4: a module's chip leads to its card. Every rule the page can name has
// an entry, empty only on purpose, and every card named is one /options
// renders with that id.
func TestEveryBlockedRuleLeadsToItsOption(t *testing.T) {
	for _, r := range shared.PacketLogRules {
		if _, ok := blockedRuleOption[r]; !ok {
			t.Errorf("rule %q has no entry in blockedRuleOption — name its /options card, or \"\" if no switch refuses it", r)
		}
	}
	for r := range blockedRuleOption {
		if r != "drop" && r != "blocklist" && blockedRuleOption[r] == "" {
			t.Errorf("rule %q is a module but leads nowhere", r)
		}
	}
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetOptions, successResp(shared.FirewallOptions{}))
	opts := doAuthRequest(t, s, "GET", "/options", nil).Body.String()
	for r, key := range blockedRuleOption {
		if key != "" && !strings.Contains(opts, `id="opt-`+key+`"`) {
			t.Errorf("rule %q leads to /options#opt-%s, which /options does not render", r, key)
		}
	}

	scan := samplePacket()
	scan.Rule = "portscan"
	if body := whyCore(t, scan, shared.Rules{}, true); !strings.Contains(body, `<a class="log-action" href="/options#opt-port_scan">Port scan</a>`) {
		t.Errorf("a port-scan row's chip does not lead to its card:\n%s", body)
	}
	drop := samplePacket()
	drop.Rule = "drop"
	if body := whyCore(t, drop, shared.Rules{}, true); !strings.Contains(body, `<span class="log-action">Default drop</span>`) {
		t.Errorf("the default-drop chip is not a plain chip:\n%s", body)
	}
}

func TestBlockedDetailsNameTheICMPType(t *testing.T) {
	ping := samplePacket()
	ping.Rule, ping.Proto, ping.DstPort, ping.SrcPort, ping.TCPFlags = "icmp_flood", "icmp", 0, 0, ""
	ping.ICMP = &shared.PacketICMP{Type: 8}
	if body := whyCore(t, ping, shared.Rules{}, true); !strings.Contains(body, `<dt>ICMP type/code</dt><dd class="pkt-flow">8/0</dd>`) {
		t.Errorf("the drill-down does not name the ICMP type:\n%s", body)
	}
}
