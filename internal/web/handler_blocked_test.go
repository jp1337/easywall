package web

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"net/url"
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
		`<span class="log-action">SSH brute force</span>`, // the row's own rule label, not the filter's <select>, which lists every label regardless of whether a row exists
		`href="/blocked?src=203.0.113.9"`,                 // clicking an address filters to it
		`id="pkt-41"`, "hx-preserve",                      // the drill-down survives the live tail
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
	if !strings.Contains(body, ">[2001:db8::1]</a>:<a href=\"/blocked?port=23\">23</a>") {
		t.Errorf("an IPv6 destination with a port is not bracketed:\n%s", body)
	}
	if !strings.Contains(body, ">198.51.100.1</a>:<a href=\"/blocked?port=22\">22</a>") {
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

	rec := doAuthRequest(t, s, "GET", "/blocked?src=203.0.113.0%2F24&port=22&proto=tcp&rule=ssh&in=eth0", nil)
	assertStatus(t, rec, http.StatusOK)
	want := shared.PacketLogFilter{Src: "203.0.113.0/24", Port: 22, Proto: "tcp", Rule: "ssh", InDev: "eth0"}
	if sent != want {
		t.Errorf("the core was asked for %+v, want %+v", sent, want)
	}
	if !strings.Contains(rec.Body.String(), `hx-get="/blocked/rows?in=eth0&amp;port=22&amp;proto=tcp&amp;rule=ssh&amp;src=203.0.113.0%2F24"`) {
		t.Error("the live tail does not carry the filter; the next swap would show everything")
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
			shared.FirewallOptions{}, `href="/options#logging"`},
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
	icmp.Proto, icmp.SrcPort, icmp.DstPort, icmp.TCPFlags, icmp.Rule = "icmp", 0, 0, "", "icmp_flood"
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
	_, s = blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{tcpRow}},
		shared.FirewallOptions{LogBlocked: true})
	if body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String(); !strings.Contains(body, `value="open"`) {
		t.Error("a TCP packet to port 22 is not offered 'open the port'")
	}
	fwd := samplePacket()
	fwd.Hook = "forward"
	_, s = blockedCore(t, shared.PacketLogResult{Listening: true, Entries: []shared.PacketLogEntry{fwd}},
		shared.FirewallOptions{LogBlocked: true})
	if body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String(); strings.Contains(body, `value="open"`) {
		t.Error("a forwarded packet is offered an input-chain port rule")
	}
}

func TestFilterQueryRoundTrips(t *testing.T) {
	f := shared.PacketLogFilter{Src: "2001:db8::/32", Port: 443, Proto: "tcp"}
	back, ok := blockedFilter(mustParseQuery(t, filterQuery(f)))
	if !ok || back != f {
		t.Errorf("round trip: %+v (ok=%v), want %+v", back, ok, f)
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
