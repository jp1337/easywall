package web

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

func samplePacket() shared.PacketLogEntry {
	return shared.PacketLogEntry{
		Seq: 41, Time: time.Now().UTC(), Rule: "ssh", Hook: "input", InDev: "eth0", Family: 4,
		Src: netip.MustParseAddr("203.0.113.9"), Dst: netip.MustParseAddr("198.51.100.1"),
		Proto: "tcp", SrcPort: 51514, DstPort: 22, TCPFlags: "SYN", TTL: 57, CtState: "new",
	}
}

func blockedCore(t *testing.T, res shared.PacketLogResult, opts shared.FirewallOptions) (*fakeCore, *Server) {
	t.Helper()
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetPacketLog, successResp(res))
	fc.SetResponse(shared.CmdGetOptions, successResp(opts))
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{}))
	// The header's staged count now comes from buildPreview, which also reads
	// these two — stubbed here so every /blocked test gets a complete preview
	// rather than logging "the apply preview has no configuration half" on
	// every single request.
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))
	fc.SetResponse(shared.CmdGetAppliedConfig, successResp(shared.AppliedConfigResult{}))
	return fc, s
}

func TestBlocked_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	assertRedirect(t, doRequest(s, "GET", "/blocked", nil), "/login")
	assertRedirect(t, doRequest(s, "GET", "/blocked/rows", nil), "/login")
}

func TestBlocked_RendersARow(t *testing.T) {
	_, s := blockedCore(t, shared.PacketLogResult{Listening: true, Held: 1, Matched: 1,
		Entries: []shared.PacketLogEntry{samplePacket()}}, shared.FirewallOptions{SSHBruteForceLog: true})

	rec := doAuthRequest(t, s, "GET", "/blocked", nil)
	assertStatus(t, rec, http.StatusOK)
	body := rec.Body.String()
	for _, want := range []string{
		"203.0.113.9", "198.51.100.1", `href="/blocked?port=22"`, "eth0",
		`<a class="log-action" href="/options#opt-ssh_brute_force">SSH brute force</a>`, // the row's own rule label, not the filter's <select>, which lists every label regardless of whether a row exists
		`href="/blocked?src=203.0.113.9"`,                                               // clicking an address filters to it
		`id="pkt-41"`, "hx-preserve",                                                    // the drill-down survives the live tail
		`hx-trigger="every 5s"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %q", want)
		}
	}
}

// RFC 3986 (and net.JoinHostPort, for the same reason): an IPv6 host takes
// brackets once a port follows it, or "2001:db8::1:23" reads as seven groups
// instead of six-plus-a-port. IPv4 has no colon to disambiguate and stays
// exactly as it was.
func TestBlocked_BracketsAnIPv6DestinationWhenAPortFollows(t *testing.T) {
	v6 := samplePacket()
	v6.Seq = 42
	v6.Family = 6
	v6.Src = netip.MustParseAddr("2001:db8:bad::17")
	v6.Dst = netip.MustParseAddr("2001:db8::1")
	v6.DstPort = 23

	_, s := blockedCore(t, shared.PacketLogResult{Listening: true, Held: 2, Matched: 2,
		Entries: []shared.PacketLogEntry{samplePacket(), v6}}, shared.FirewallOptions{SSHBruteForceLog: true})

	rec := doAuthRequest(t, s, "GET", "/blocked", nil)
	assertStatus(t, rec, http.StatusOK)
	body := rec.Body.String()
	if !strings.Contains(body, ">[2001:db8::1]</a><a href=\"/blocked?port=23\">:23</a>") {
		t.Errorf("an IPv6 destination with a port is not bracketed:\n%s", body)
	}
	if !strings.Contains(body, ">198.51.100.1</a><a href=\"/blocked?port=22\">:22</a>") {
		t.Errorf("an IPv4 destination with a port gained brackets it should not have:\n%s", body)
	}
}

// The filter reaches the core as typed fields, and the URL is its only state:
// the live tail and every link carry the same query back.
func TestBlocked_TheFilterGoesToTheCoreAndStaysInTheURL(t *testing.T) {
	fc, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{}},
		shared.FirewallOptions{LogBlocked: true})
	var sent shared.PacketLogFilter
	fc.OnCommand(shared.CmdGetPacketLog, func(c shared.Command) { _ = json.Unmarshal(c.Payload, &sent) })

	rec := doAuthRequest(t, s, "GET", "/blocked?in=eth0&port=22&proto=tcp&rule=ssh&src=203.0.113.0%2F24", nil)
	assertStatus(t, rec, http.StatusOK)
	want := shared.PacketLogFilter{Src: "203.0.113.0/24", Port: 22, Proto: "tcp", Rule: "ssh", InDev: "eth0"}
	if sent != want {
		t.Errorf("the core was asked for %+v, want %+v", sent, want)
	}
	if !strings.Contains(rec.Body.String(), `hx-get="/blocked/rows?in=eth0&amp;port=22&amp;proto=tcp&amp;rule=ssh&amp;src=203.0.113.0%2F24"`) {
		t.Error("the live tail does not carry the filter; the next swap would show everything")
	}
}

func TestBlockedRedirectsToTheCanonicalQuery(t *testing.T) {
	_, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{}},
		shared.FirewallOptions{LogBlocked: true})
	rec := doAuthRequest(t, s, "GET", "/blocked?src=&dst=&port=22&proto=&rule=&in=", nil)
	assertRedirect(t, rec, "/blocked?port=22")
	rec = doAuthRequest(t, s, "GET", "/blocked?src=&dst=", nil)
	assertRedirect(t, rec, "/blocked")
	assertStatus(t, doAuthRequest(t, s, "GET", "/blocked?port=22", nil), http.StatusOK)
	// The spelling html/template gives an IPv6 link is already canonical.
	assertStatus(t, doAuthRequest(t, s, "GET", "/blocked?src=2001%3adb8%3a%3a1", nil), http.StatusOK)
}

func TestBlockedKeepsAnUnreadableFilterInTheForm(t *testing.T) {
	_, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{samplePacket()}},
		shared.FirewallOptions{LogBlocked: true})
	body := doAuthRequest(t, s, "GET", "/blocked?src=10.0.0.0%2F33&port=22", nil).Body.String()
	if !strings.Contains(body, `value="10.0.0.0/33"`) {
		t.Error("the field no longer shows what was typed")
	}
	if !regexp.MustCompile(`name="src"[^>]*aria-invalid="true"|aria-invalid="true"[^>]*name="src"`).MatchString(body) {
		t.Error("the field that could not be read is not marked")
	}
	if !regexp.MustCompile(`name="src"[^>]*class="[^"]*\bis-error\b|class="[^"]*\bis-error\b[^"]*"[^>]*name="src"`).MatchString(body) {
		t.Error("the field that could not be read does not carry the is-error class")
	}
	if regexp.MustCompile(`name="port"[^>]*aria-invalid="true"`).MatchString(body) {
		t.Error("a readable field is marked invalid")
	}
}

func TestBlockedHeaderCountsLikeTheApplyScreen(t *testing.T) {
	fc, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{}},
		shared.FirewallOptions{LogBlocked: true, Fragments: true})
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Staged: shared.Rules{Whitelist: []string{"192.0.2.9"}},
	}))
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{}))
	fc.SetResponse(shared.CmdGetAppliedConfig, successResp(shared.AppliedConfigResult{
		Recorded: true,
		Config:   shared.AppliedConfig{Firewall: shared.FirewallOptions{LogBlocked: true}},
	}))
	body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String()
	if !strings.Contains(body, "2 staged") {
		t.Error("one rule change and one configuration change must read as 2 staged, as /apply counts them")
	}
}

// One number, one source: /apply renders Preview.Total even when Incomplete
// is true (buildPreview already sets Total to the rule count before it
// returns on an unreadable configuration half), so /blocked must show the
// same number rather than hide it because one of the five reads failed.
func TestBlockedHeaderCountsEvenWhenThePreviewIsIncomplete(t *testing.T) {
	fc, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{}},
		shared.FirewallOptions{LogBlocked: true})
	fc.SetResponse(shared.CmdGetRules, successResp(shared.RulesState{
		Staged: shared.Rules{Whitelist: []string{"192.0.2.9"}},
	}))
	fc.SetResponse(shared.CmdGetSettings, errorRespFor("unavailable"))
	body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String()
	if !strings.Contains(body, "1 staged") {
		t.Error("one staged rule change must read as 1 staged even when GetSettings fails and the preview is incomplete")
	}
}

// Review Focus 3. A filter that does not parse is ignored out loud: the page
// renders the whole log and says the filter was not applied, rather than 500ing
// or showing an empty table that reads as "nothing was refused".
func TestBlockedIgnoresAFilterItCannotRead(t *testing.T) {
	for _, q := range []string{"port=99999", "port=0", "src=not-an-ip", "rule=%3Cscript%3E", "in=eth0%3Brm"} {
		fc, s := blockedCore(t, shared.PacketLogResult{Listening: true, Held: 1, Matched: 1,
			Entries: []shared.PacketLogEntry{samplePacket()}}, shared.FirewallOptions{LogBlocked: true})
		var sent shared.PacketLogFilter
		fc.OnCommand(shared.CmdGetPacketLog, func(c shared.Command) { _ = json.Unmarshal(c.Payload, &sent) })

		rec := doAuthRequest(t, s, "GET", "/blocked?"+q, nil)
		assertStatus(t, rec, http.StatusOK)
		body := rec.Body.String()
		if !strings.Contains(body, "could not be read") {
			t.Errorf("%s: the page does not say the filter was ignored", q)
		}
		if !strings.Contains(body, "203.0.113.9") {
			t.Errorf("%s: the unfiltered log is not shown", q)
		}
		if sent != (shared.PacketLogFilter{}) {
			t.Errorf("%s: the core was sent %+v; a filter the page refused must not reach it", q, sent)
		}
	}
}

func TestBlocked_EmptyStates(t *testing.T) {
	for _, tc := range []struct {
		name string
		res  shared.PacketLogResult
		opts shared.FirewallOptions
		want string
	}{
		{"nothing switched on", shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{}},
			shared.FirewallOptions{}, `href="/options#opt-log_blocked_connections"`},
		{"on, and quiet", shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{}},
			shared.FirewallOptions{LogBlocked: true}, "Nothing has been refused"},
		{"not listening", shared.PacketLogResult{Group: 12227, Reason: "bind NFLOG group 12227: device or resource busy",
			Entries: []shared.PacketLogEntry{}}, shared.FirewallOptions{LogBlocked: true}, "device or resource busy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, s := blockedCore(t, tc.res, tc.opts)
			body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String()
			if !strings.Contains(body, tc.want) {
				t.Errorf("the page does not contain %q", tc.want)
			}
		})
	}
}

// Final review, Important 1. GetPacketLog failing is not "nothing was
// refused", and GetOptions failing is not "logging is off" — both are the
// core not answering, which the handler's own comment says should read as
// nothing rather than something false.
func TestBlockedRows_CoreErrDoesNotClaimLoggingIsOff(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetPacketLog, errorRespFor("unavailable"))
	fc.SetResponse(shared.CmdGetOptions, successResp(shared.FirewallOptions{}))

	body := doAuthRequest(t, s, "GET", "/blocked/rows", nil).Body.String()
	if strings.Contains(body, "Every log switch is off") {
		t.Error("the live tail claims logging is off when the core could not be asked at all")
	}
	if !strings.Contains(body, "unavailable") {
		t.Errorf("the live tail does not say the core could not be read:\n%s", body)
	}
}

// The live tail asks GetPacketLog and GetOptions independently of the page
// load; GetOptions failing while GetPacketLog succeeds (even with an empty
// log) must not read as "logging is off" in the tail's own fragment either.
func TestBlockedRows_OptionsErrDoesNotClaimLoggingIsOff(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetPacketLog, successResp(shared.PacketLogResult{
		Listening: true, Entries: []shared.PacketLogEntry{}}))
	fc.SetResponse(shared.CmdGetOptions, errorRespFor("unavailable"))

	body := doAuthRequest(t, s, "GET", "/blocked/rows", nil).Body.String()
	if strings.Contains(body, "Every log switch is off") {
		t.Error("the live tail claims logging is off when GetOptions failed")
	}
	if !strings.Contains(body, "could not be read") {
		t.Errorf("the live tail does not say whether anything is logged is unknown:\n%s", body)
	}
}

func TestBlocked_LoggingUnknownDoesNotClaimItIsOff(t *testing.T) {
	fc, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{}},
		shared.FirewallOptions{})
	fc.SetResponse(shared.CmdGetOptions, errorRespFor("unavailable"))

	body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String()
	if strings.Contains(body, "Every log switch is off") {
		t.Error("the page claims logging is off when GetOptions failed; unknown must not read as false")
	}
	if !strings.Contains(body, "could not be read") {
		t.Errorf("the page does not say whether anything is logged is unknown:\n%s", body)
	}
}

// Final review, Important 2. A bind that never succeeded still has the
// kernel-log fallback; a listener that ran and then died does not — the two
// must not share a sentence.
func TestBlocked_NotListeningTellsBindApartFromStopped(t *testing.T) {
	for _, tc := range []struct {
		name           string
		res            shared.PacketLogResult
		want, mustLack string
	}{
		{"bind never succeeded",
			shared.PacketLogResult{Group: 12227, Reason: "device or resource busy"},
			"kernel log instead", "restarts"},
		{"ran, then stopped",
			shared.PacketLogResult{Group: 12227, Reason: "connection reset by peer", Stopped: true},
			"restarts", "kernel log instead"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, s := blockedCore(t, tc.res, shared.FirewallOptions{LogBlocked: true})
			body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String()
			if !strings.Contains(body, tc.want) {
				t.Errorf("%s: the page does not say %q:\n%s", tc.name, tc.want, body)
			}
			if strings.Contains(body, tc.mustLack) {
				t.Errorf("%s: the page wrongly says %q", tc.name, tc.mustLack)
			}
		})
	}
}

func TestBlocked_CoreDownStillRenders(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetPacketLog, errorRespFor("unavailable"))
	rec := doAuthRequest(t, s, "GET", "/blocked", nil)
	assertStatus(t, rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), "alert-crit") {
		t.Error("a core that did not answer is not reported")
	}
}

func TestBlockedRows_IsAFragment(t *testing.T) {
	_, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{samplePacket()}},
		shared.FirewallOptions{LogBlocked: true})
	body := doAuthRequest(t, s, "GET", "/blocked/rows?port=22", nil).Body.String()
	if strings.Contains(body, "<html") || !strings.Contains(body, "203.0.113.9") {
		t.Errorf("/blocked/rows is not the rows fragment:\n%s", body)
	}
}

// The live tail is its own request and is handed nothing handleBlocked
// already worked out, so it has to ask the core the same two questions again
// to pick the same empty-state message — this is what proves it does.
func TestBlockedRows_EmptyStateMatchesThePage(t *testing.T) {
	_, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{}},
		shared.FirewallOptions{LogBlocked: true})
	body := doAuthRequest(t, s, "GET", "/blocked/rows", nil).Body.String()
	if !strings.Contains(body, "Nothing has been refused") {
		t.Errorf("the live tail's own empty state does not match the page's:\n%s", body)
	}
}

func TestBlockedOffersOpenPortOnlyWithAPort(t *testing.T) {
	icmp := samplePacket()
	icmp.Proto, icmp.SrcPort, icmp.DstPort, icmp.TCPFlags, icmp.Rule = "icmp", 0, 0, "", "drop"
	_, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{icmp}},
		shared.FirewallOptions{ICMPFloodLog: true})
	body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String()
	// Not ":0" — every timestamp on the page contains that.
	if strings.Contains(body, `port=0"`) {
		t.Error("an ICMP packet is rendered with port 0")
	}
	if strings.Contains(body, `value="open"`) {
		t.Error("an ICMP packet is offered 'open the port'")
	}
	tcpRow := samplePacket()
	tcpRow.Rule = "drop"
	_, s = blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{tcpRow}},
		shared.FirewallOptions{LogBlocked: true})
	if body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String(); !strings.Contains(body, `value="open"`) {
		t.Error("a TCP packet to port 22 is not offered 'open the port'")
	}
	fwd := samplePacket()
	fwd.Hook = "forward"
	fwd.Rule = "drop"
	_, s = blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{fwd}},
		shared.FirewallOptions{LogBlocked: true})
	if body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String(); strings.Contains(body, `value="open"`) {
		t.Error("a forwarded packet is offered an input-chain port rule")
	}
}

func TestBlockedOffersOnlyRemedies(t *testing.T) {
	scan := samplePacket()
	scan.Rule, scan.DstPort = "portscan", 3389
	bl := samplePacket()
	bl.Rule = "blacklist"
	drop := samplePacket()
	drop.Rule = "drop"
	for _, tc := range []struct {
		name        string
		e           shared.PacketLogEntry
		want, avoid []string
	}{
		{"port scan", scan, []string{`value="blacklist"`}, []string{`value="whitelist"`, `value="open"`, `href="/blacklist"`}},
		{"blacklist", bl, []string{`href="/blacklist"`}, []string{`value="whitelist"`, `value="blacklist"`, `value="open"`}},
		{"default drop", drop, []string{`value="whitelist"`, `value="blacklist"`, `value="open"`}, []string{`href="/blacklist"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{tc.e}},
				shared.FirewallOptions{LogBlocked: true})
			body := doAuthRequest(t, s, "GET", "/blocked/rows", nil).Body.String()
			for _, w := range tc.want {
				if !strings.Contains(body, w) {
					t.Errorf("missing %s", w)
				}
			}
			for _, a := range tc.avoid {
				if strings.Contains(body, a) {
					t.Errorf("offers %s, which could not have let this packet through", a)
				}
			}
		})
	}
}

// A forwarded row offers none of the three actions and no blacklist link
// (Remedies() is the zero value); the actions cell must not render empty —
// a mobile "Actions" label over nothing reads as a bug.
func TestBlockedForwardedRowShowsADashNotAnEmptyCell(t *testing.T) {
	fwd := samplePacket()
	fwd.Hook, fwd.Rule = "forward", "drop"
	_, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{fwd}},
		shared.FirewallOptions{LogBlocked: true})
	body := doAuthRequest(t, s, "GET", "/blocked/rows", nil).Body.String()
	for _, avoid := range []string{`value="whitelist"`, `value="blacklist"`, `value="open"`, `href="/blacklist"`, `<form`} {
		if strings.Contains(body, avoid) {
			t.Errorf("forwarded row offers %s, which could not have let it through", avoid)
		}
	}
	if !strings.Contains(body, `<span class="text-ink-subtle">—</span>`) {
		t.Error("forwarded row's actions cell has no control and no placeholder dash")
	}
}

func TestFilterQueryRoundTrips(t *testing.T) {
	f := shared.PacketLogFilter{Src: "2001:db8::/32", Port: 443, Proto: "tcp"}
	back, bad := blockedFilter(mustParseQuery(t, filterQuery(f)))
	if bad != "" || back != f {
		t.Errorf("round trip: %+v (bad=%q), want %+v", back, bad, f)
	}
}

// The rule labels are asked for through printf, which the template-key guard
// cannot see. Derived from shared.PacketLogRules, so a rule added there without
// a label fails here instead of rendering "blocked_rule_newthing".
func TestEveryPacketLogRuleIsLabelled(t *testing.T) {
	for _, lang := range []string{"en", "de"} {
		ids := localeIDs(t, lang)
		for _, r := range append(append([]string{}, shared.PacketLogRules...), shared.PacketLogRuleOther) {
			if !ids["blocked_rule_"+r] {
				t.Errorf("locales/%s.json has no blocked_rule_%s", lang, r)
			}
		}
	}
}

func mustParseQuery(t *testing.T, q string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(q)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
