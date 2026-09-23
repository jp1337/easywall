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
	for _, r := range []string{"ssh", "portscan", "drop", "blacklist"} {
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
