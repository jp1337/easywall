package web

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// The public demo is where most people ever see this page. It must show rows,
// every rule it labels, and new packets arriving — and only documentation
// addresses, so nothing on it points at a real network.
func TestDemoPacketLog(t *testing.T) {
	start := time.Now().Add(-10 * time.Minute)
	early := demoPacketLog(start, start.Add(time.Minute))
	late := demoPacketLog(start, start.Add(10*time.Minute))
	if len(early) == 0 || len(late) <= len(early) {
		t.Fatalf("%d entries after a minute, %d after ten; the stream must grow", len(early), len(late))
	}
	rules := map[string]bool{}
	docs := []string{"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", "2001:db8::/32"}
	for i, e := range late {
		rules[e.Rule] = true
		if !shared.InAnyEntry(e.Src, docs) || !shared.InAnyEntry(e.Dst, docs) {
			t.Errorf("entry %d uses %s → %s, outside the documentation ranges", i, e.Src, e.Dst)
		}
		if i > 0 && e.Seq >= late[i-1].Seq {
			t.Errorf("not newest first at %d", i)
		}
	}
	for _, r := range []string{"ssh", "portscan", "drop", "blocklist"} {
		if !rules[r] {
			t.Errorf("the demo never shows a %s entry", r)
		}
	}
}

// A demo left running for days must keep its live tail moving, not cap out
// once the window (500 entries at one every seven seconds, about an hour)
// fills and stop advancing.
func TestDemoPacketLog_KeepsMovingPastTheWindow(t *testing.T) {
	start := time.Now().Add(-48 * time.Hour)
	if demoPacketLog(start, start.Add(24*time.Hour))[0].Seq <= demoPacketLog(start, start.Add(23*time.Hour))[0].Seq {
		t.Error("the newest entry's Seq did not advance between hour 23 and hour 24")
	}
}

func TestDemoAnswersGetPacketLogFiltered(t *testing.T) {
	d := newDemoState()
	payload, _ := json.Marshal(shared.PacketLogFilter{Rule: "ssh"})
	resp := d.Send(shared.Command{Type: shared.CmdGetPacketLog, Payload: payload})
	var res shared.PacketLogResult
	if !resp.Success || json.Unmarshal(resp.Data, &res) != nil {
		t.Fatalf("%+v", resp)
	}
	if !res.Listening || len(res.Entries) == 0 {
		t.Fatalf("the demo reports %+v", res)
	}
	for _, e := range res.Entries {
		if e.Rule != "ssh" {
			t.Errorf("filter ignored: %+v", e)
		}
	}
}

// A demo row blocked by "default drop" from an address the demo's own
// blocklist already covers is a firewall that could not have produced it, and
// every blocklist click then answers "already there".
func TestDemoPacketSourcesAgreeWithTheDemoLists(t *testing.T) {
	d := newDemoState()
	bl, wl := d.rules.Current.Blocklist, d.rules.Current.Allowlist
	for _, e := range demoShapes {
		inBL := shared.InAnyEntry(e.Src, bl)
		switch {
		case e.Rule == "blocklist" && !inBL:
			t.Errorf("%s is logged as a blocklist hit but the demo blocklist does not cover it", e.Src)
		case e.Rule != "blocklist" && inBL:
			t.Errorf("%s (%s) is covered by the demo blocklist, which drops it before %s could", e.Src, e.Rule, e.Rule)
		case (e.Rule == "drop" || e.Rule == "bogon" || e.Rule == shared.PacketLogRuleOther) && shared.InAnyEntry(e.Src, wl):
			// Modules run before the allowlist, so an allowlisted source can be
			// logged by ssh or portscan; it cannot be by what runs after.
			t.Errorf("%s is allowlisted in the demo, so %s — which runs after the allowlist — could not have refused it", e.Src, e.Rule)
		}
	}
}

// The real core records an ICMP packet's type since 2.22; a demo row without
// one would show the public page a gap the product does not have. Asked
// through GET_PACKET_LOG, so the field is proven to survive the JSON too.
func TestDemoICMPRowsCarryTheirType(t *testing.T) {
	// icmp only: the demo has no ICMPv6 shape (grep icmpv6 democlient.go is
	// empty), so a loop over both protocols passes the icmpv6 half vacuously
	// — no entries, so the body under it never runs.
	d := newDemoState()
	payload, _ := json.Marshal(shared.PacketLogFilter{Proto: "icmp"})
	var res shared.PacketLogResult
	if resp := d.Send(shared.Command{Type: shared.CmdGetPacketLog, Payload: payload}); !resp.Success ||
		json.Unmarshal(resp.Data, &res) != nil {
		t.Fatalf("%+v", resp)
	}
	if len(res.Entries) == 0 {
		t.Fatal("no demo icmp rows — the checks below would pass vacuously")
	}
	for _, e := range res.Entries {
		if e.ICMP == nil {
			t.Errorf("a demo icmp row (%s from %s) has no ICMP type", e.Rule, e.Src)
		} else if e.Rule == "icmp_flood" && e.ICMP.Type != 8 {
			t.Errorf("an ICMP-flood row is type %d; the module only meters echo requests", e.ICMP.Type)
		}
	}
}
