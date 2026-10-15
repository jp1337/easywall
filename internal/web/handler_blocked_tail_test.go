package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// tailRing is the ring as root01xvp held it on 2026-10-13: more entries than
// the page shows, newest first, so the oldest row falls off with every new one.
func tailRing(newest uint64, n int) []shared.PacketLogEntry {
	out := make([]shared.PacketLogEntry, 0, n)
	for i := range n {
		e := samplePacket()
		e.Seq = newest - uint64(i)
		out = append(out, e)
	}
	return out
}

func tailGet(t *testing.T, ring []shared.PacketLogEntry, path string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	_, s := blockedCore(t, shared.PacketLogResult{Listening: true, Held: 311, Matched: len(ring), Entries: ring},
		shared.FirewallOptions{LogBlocked: true})
	rec := doAuthRequest(t, s, "GET", path, nil)
	return rec, rec.Body.String()
}

func TestBlockedRows_WithoutAfterSendsTheWholeView(t *testing.T) {
	res, body := tailGet(t, tailRing(311, 200), "/blocked/rows")
	if res.Code != http.StatusOK || res.Header().Get("HX-Reswap") != "innerHTML" {
		t.Fatalf("status %d, HX-Reswap %q; want 200 and innerHTML", res.Code, res.Header().Get("HX-Reswap"))
	}
	if n := strings.Count(body, `id="pkt-`); n != 200 {
		t.Errorf("%d rows; want all 200", n)
	}
}

func TestBlockedRows_AfterSendsOnlyTheNewerRowsOnTop(t *testing.T) {
	res, body := tailGet(t, tailRing(311, 200), "/blocked/rows?after=308")
	if res.Code != http.StatusOK || res.Header().Get("HX-Reswap") != "afterbegin" {
		t.Fatalf("status %d, HX-Reswap %q; want 200 and afterbegin", res.Code, res.Header().Get("HX-Reswap"))
	}
	for _, want := range []string{`id="pkt-311"`, `id="pkt-310"`, `id="pkt-309"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the delta lacks %s", want)
		}
	}
	if strings.Contains(body, `id="pkt-308"`) || strings.Count(body, `id="pkt-`) != 3 {
		t.Errorf("the delta carries rows the page already has:\n%s", body)
	}
	if strings.Index(body, "pkt-311") > strings.Index(body, "pkt-309") {
		t.Error("the delta is not newest first")
	}
}

func TestBlockedRows_NothingNewIsNoContent(t *testing.T) {
	res, body := tailGet(t, tailRing(311, 200), "/blocked/rows?after=311")
	if res.Code != http.StatusNoContent || body != "" {
		t.Errorf("status %d with %d bytes; want 204 and an empty body", res.Code, len(body))
	}
}

func TestBlockedRows_ARenumberedRingSendsTheWholeView(t *testing.T) {
	res, body := tailGet(t, tailRing(12, 12), "/blocked/rows?after=311")
	if res.Header().Get("HX-Reswap") != "innerHTML" || strings.Count(body, `id="pkt-`) != 12 {
		t.Errorf("HX-Reswap %q with %d rows; want innerHTML and all 12", res.Header().Get("HX-Reswap"), strings.Count(body, `id="pkt-`))
	}
}

// A page that holds rows, asking after an empty ring, must be told the ring is
// empty; a 204 would leave the old rows on screen for as long as it is open.
func TestBlockedRows_AnEmptyRingSendsTheWholeView(t *testing.T) {
	res, body := tailGet(t, nil, "/blocked/rows?after=311")
	if res.Code != http.StatusOK || res.Header().Get("HX-Reswap") != "innerHTML" {
		t.Fatalf("status %d, HX-Reswap %q; want 200 and innerHTML", res.Code, res.Header().Get("HX-Reswap"))
	}
	if !strings.Contains(body, "Nothing has been refused since") {
		t.Errorf("the empty ring does not say so:\n%s", body)
	}
}

func TestBlockedRows_AnUnreadableAfterSendsTheWholeView(t *testing.T) {
	res, _ := tailGet(t, tailRing(311, 5), "/blocked/rows?after=x")
	if res.Header().Get("HX-Reswap") != "innerHTML" {
		t.Errorf("HX-Reswap %q; want innerHTML", res.Header().Get("HX-Reswap"))
	}
}

func TestBlockedRows_IsNeverStored(t *testing.T) {
	for _, path := range []string{"/blocked/rows", "/blocked/rows?after=311", "/blocked/rows?after=308"} {
		res, _ := tailGet(t, tailRing(311, 200), path)
		if got := res.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control %q; want no-store", path, got)
		}
	}
}

// rulesNow costs two socket round trips; a delta without a default-drop row
// must not pay them. With the lookup on and a country on the new rows, the
// Block button may ask GET_RULES once — and no more.
func TestBlockedRows_ADeltaWithoutADropRowAsksNoRules(t *testing.T) {
	for name, on := range map[string]bool{"lookup off": false, "lookup on": true} {
		t.Run(name, func(t *testing.T) {
			ring := tailRing(311, 200)
			ring[150].Rule = "drop" // old: the page already has it
			for i := range 3 {
				ring[i].Src = netip.MustParseAddr("192.0.2.80") // DE in the sample table
			}
			fc, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: ring}, shared.FirewallOptions{LogBlocked: true})
			s.lookup = nil
			if on {
				s.lookup = lookupWith(t, sampleCountryTable(t))
			}
			var rules, applied atomic.Int32
			fc.OnCommand(shared.CmdGetRules, func(shared.Command) { rules.Add(1) })
			fc.OnCommand(shared.CmdGetAppliedConfig, func(shared.Command) { applied.Add(1) })
			body := doAuthRequest(t, s, "GET", "/blocked/rows?after=308", nil).Body.String()
			if on && !strings.Contains(body, deCode) {
				t.Fatalf("the delta's rows carry no country; the test proves nothing:\n%s", body)
			}
			want := int32(0)
			if on {
				want = 1
			}
			if got := rules.Load(); got > want {
				t.Errorf("GET_RULES asked %d times for a delta with no default-drop row; want at most %d", got, want)
			}
			if got := applied.Load(); got != 0 {
				t.Errorf("GET_APPLIED_CONFIG asked %d times for a delta with no default-drop row", got)
			}
		})
	}
}

// A delta with both a default-drop row and a country row needs the rules for
// two things — the drop reason and the Block button — and asks for them once.
func TestBlockedRows_ADropRowAndACountryRowAskTheRulesOnce(t *testing.T) {
	ring := tailRing(311, 200)
	ring[0].Rule = "drop"
	ring[1].Src = netip.MustParseAddr("192.0.2.80")
	fc, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: ring}, shared.FirewallOptions{LogBlocked: true})
	s.lookup = lookupWith(t, sampleCountryTable(t))
	var asked atomic.Int32
	fc.OnCommand(shared.CmdGetRules, func(shared.Command) { asked.Add(1) })
	body := doAuthRequest(t, s, "GET", "/blocked/rows?after=309", nil).Body.String()
	if !strings.Contains(body, deCode) {
		t.Fatalf("the delta's rows carry no country:\n%s", body)
	}
	if got := asked.Load(); got != 1 {
		t.Errorf("GET_RULES asked %d times; want exactly 1", got)
	}
}

// The full page reads the rules for the reasons, the Block buttons and the
// staged count: one GET_RULES for all three.
func TestBlocked_ThePageAsksTheRulesOnce(t *testing.T) {
	ring := tailRing(311, 5)
	ring[0].Rule = "drop"
	ring[1].Src = netip.MustParseAddr("192.0.2.80")
	fc, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: ring}, shared.FirewallOptions{LogBlocked: true})
	s.lookup = lookupWith(t, sampleCountryTable(t))
	var asked atomic.Int32
	fc.OnCommand(shared.CmdGetRules, func(shared.Command) { asked.Add(1) })
	body := doAuthRequest(t, s, "GET", "/blocked", nil).Body.String()
	if !strings.Contains(body, deCode) {
		t.Fatalf("the page's rows carry no country:\n%s", body)
	}
	if got := asked.Load(); got != 1 {
		t.Errorf("GET_RULES asked %d times; want exactly 1", got)
	}
}

// A tail whose packet log could not be read still renders under the page's
// head, and that head has the Country column while the lookup is on.
func TestBlockedRows_AFailedTailSpansTheCountryColumn(t *testing.T) {
	fc, s := blockedCore(t, shared.PacketLogResult{}, shared.FirewallOptions{LogBlocked: true})
	s.lookup = lookupWith(t, sampleCountryTable(t))
	fc.SetResponse(shared.CmdGetPacketLog, shared.Response{Success: false, Error: "timeout"})
	body := doAuthRequest(t, s, "GET", "/blocked/rows?after=5", nil).Body.String()
	if !strings.Contains(body, `colspan="7"`) {
		t.Errorf("the failed tail does not span the 7-column head:\n%s", body)
	}
}

// The filter must reach the core on a delta request too.
func TestBlockedRows_AFilterWithAfterStillReachesTheCore(t *testing.T) {
	fc, s := blockedCore(t, shared.PacketLogResult{Listening: true, Entries: tailRing(311, 5)}, shared.FirewallOptions{LogBlocked: true})
	var port atomic.Int32
	fc.OnCommand(shared.CmdGetPacketLog, func(c shared.Command) {
		var f shared.PacketLogFilter
		_ = json.Unmarshal(c.Payload, &f)
		port.Store(int32(f.Port))
	})
	doAuthRequest(t, s, "GET", "/blocked/rows?port=22&after=308", nil)
	if got := port.Load(); got != 22 {
		t.Errorf("the core saw port %d; want 22", got)
	}
}
